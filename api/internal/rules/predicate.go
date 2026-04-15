// Package rules implements the ADR 003 D2 generalized match → action
// engine. Predicates are JSONB documents stored in correlation_rules.body
// (behind a `collection_id` indirection post-ADR-006 D6) and in
// collections.predicate.
//
// Post-ADR 006 D5 (finding-scope amendment) the evaluator dispatches on
// the collection's scope to resolve the right field set:
//
//	asset    → *model.Asset
//	endpoint → an EndpointView (asset + endpoint columns joined)
//	finding  → *model.Finding   (placeholder until P3 lands findings)
//
// The operator set (`$and`, `$or`, `$not`, `$eq`, `$ne`, `$in`, `$cidr`,
// `$regex`, `$gt/gte/lt/lte`, `$exists`) is the same as the pre-refactor
// evaluator — only the field lookup differs per scope.
package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
)

// ErrNotImplemented is retained as the sentinel for subject-mismatch
// on finding-scope evaluation (e.g., nil subject). P4 wires the real
// finding-scope lookup below; callers that pass the wrong subject type
// still get a typed error via the scope-dispatcher.
var ErrNotImplemented = errors.New("finding-scope predicate received unsupported subject")

// Scope is the runtime classification of a collection predicate target.
// Mirrors model.CollectionScope* constants.
type Scope string

const (
	ScopeAsset    Scope = "asset"
	ScopeEndpoint Scope = "endpoint"
	ScopeFinding  Scope = "finding"
)

// EndpointView is the canonical "flat endpoint" shape the evaluator sees
// when scope=endpoint. It joins a host-level *model.Asset with one of
// its *model.AssetEndpoint rows so predicates can reference fields from
// both sides (e.g. `ip`, `hostname` and `service`, `version`) without
// caring about the table boundary.
type EndpointView struct {
	Asset    *model.Asset
	Endpoint *model.AssetEndpoint
}

// Match evaluates a predicate JSONB against a subject in the given scope.
// `subject` is typed based on scope:
//
//	ScopeAsset    → *model.Asset
//	ScopeEndpoint → EndpointView (value, not pointer) or *EndpointView
//	ScopeFinding  → *model.Finding (currently returns ErrNotImplemented)
//
// An empty predicate matches anything. Malformed predicates return
// (false, error) — callers typically log and skip the rule.
func Match(predicate json.RawMessage, scope Scope, subject any) (bool, error) {
	if len(predicate) == 0 {
		return true, nil
	}
	var node any
	if err := json.Unmarshal(predicate, &node); err != nil {
		return false, fmt.Errorf("invalid predicate: %w", err)
	}
	lookup, err := lookupFor(scope, subject)
	if err != nil {
		return false, err
	}
	return evalNode(node, lookup)
}

// fieldLookup resolves a dotted path into the subject and reports
// (value, present).
type fieldLookup func(path string) (any, bool)

func lookupFor(scope Scope, subject any) (fieldLookup, error) {
	switch scope {
	case ScopeAsset:
		a, ok := subject.(*model.Asset)
		if !ok || a == nil {
			return nil, fmt.Errorf("asset-scope predicate expects *model.Asset, got %T", subject)
		}
		return assetLookup(a), nil
	case ScopeEndpoint:
		var v EndpointView
		switch s := subject.(type) {
		case EndpointView:
			v = s
		case *EndpointView:
			if s == nil {
				return nil, fmt.Errorf("endpoint-scope predicate got nil EndpointView")
			}
			v = *s
		default:
			return nil, fmt.Errorf("endpoint-scope predicate expects EndpointView, got %T", subject)
		}
		if v.Asset == nil || v.Endpoint == nil {
			return nil, fmt.Errorf("endpoint-scope EndpointView must carry both Asset and Endpoint")
		}
		return endpointLookup(v), nil
	case ScopeFinding:
		f, ok := subject.(*model.Finding)
		if !ok || f == nil {
			return nil, fmt.Errorf("finding-scope predicate expects *model.Finding, got %T: %w", subject, ErrNotImplemented)
		}
		return findingLookup(f), nil
	default:
		return nil, fmt.Errorf("unsupported predicate scope: %s", scope)
	}
}

// --- operator tree ------------------------------------------------

func evalNode(node any, get fieldLookup) (bool, error) {
	obj, ok := node.(map[string]any)
	if !ok {
		return false, fmt.Errorf("predicate node must be object, got %T", node)
	}
	if v, ok := obj["$and"]; ok {
		return evalAnd(v, get)
	}
	if v, ok := obj["$or"]; ok {
		return evalOr(v, get)
	}
	if v, ok := obj["$not"]; ok {
		ok, err := evalNode(v, get)
		return !ok, err
	}
	for field, val := range obj {
		ok, err := evalTerm(field, val, get)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalAnd(node any, get fieldLookup) (bool, error) {
	arr, ok := node.([]any)
	if !ok {
		return false, fmt.Errorf("$and expects array")
	}
	for _, child := range arr {
		ok, err := evalNode(child, get)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalOr(node any, get fieldLookup) (bool, error) {
	arr, ok := node.([]any)
	if !ok {
		return false, fmt.Errorf("$or expects array")
	}
	for _, child := range arr {
		ok, err := evalNode(child, get)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func evalTerm(field string, val any, get fieldLookup) (bool, error) {
	if op, ok := val.(map[string]any); ok && hasOperatorKey(op) {
		return evalOperator(field, op, get)
	}
	got, present := get(field)
	if !present {
		return false, nil
	}
	return scalarEq(val, got), nil
}

func hasOperatorKey(m map[string]any) bool {
	for k := range m {
		if strings.HasPrefix(k, "$") {
			return true
		}
	}
	return false
}

func evalOperator(field string, op map[string]any, get fieldLookup) (bool, error) {
	got, present := get(field)
	for k, v := range op {
		switch k {
		case "$exists":
			want, _ := v.(bool)
			if present != want {
				return false, nil
			}
		case "$eq":
			if !present || !scalarEq(v, got) {
				return false, nil
			}
		case "$ne":
			if present && scalarEq(v, got) {
				return false, nil
			}
		case "$in":
			arr, ok := v.([]any)
			if !ok {
				return false, fmt.Errorf("$in expects array")
			}
			if !present || !anyScalarEq(arr, got) {
				return false, nil
			}
		case "$cidr":
			s, _ := v.(string)
			if !present || !cidrContains(s, got) {
				return false, nil
			}
		case "$regex":
			s, _ := v.(string)
			if !present || !regexMatch(s, got) {
				return false, nil
			}
		case "$gt", "$lt", "$gte", "$lte":
			if !present || !numCompare(k, v, got) {
				return false, nil
			}
		default:
			return false, fmt.Errorf("unknown operator: %s", k)
		}
	}
	return true, nil
}

// --- per-scope lookups --------------------------------------------

func assetLookup(a *model.Asset) fieldLookup {
	return func(path string) (any, bool) {
		switch path {
		case "ip", "primary_ip":
			return derefString(a.PrimaryIP)
		case "hostname":
			return derefString(a.Hostname)
		case "source":
			return a.Source, a.Source != ""
		case "environment":
			return derefString(a.Environment)
		case "resource_type":
			return a.ResourceType, a.ResourceType != ""
		case "first_seen":
			return a.FirstSeen.Format(time.RFC3339), true
		case "last_seen":
			return a.LastSeen.Format(time.RFC3339), true
		}
		return nil, false
	}
}

func endpointLookup(v EndpointView) fieldLookup {
	a, e := v.Asset, v.Endpoint
	return func(path string) (any, bool) {
		// Host-level fields.
		switch path {
		case "ip", "primary_ip":
			return derefString(a.PrimaryIP)
		case "hostname":
			return derefString(a.Hostname)
		case "source":
			return a.Source, a.Source != ""
		case "environment":
			return derefString(a.Environment)
		case "resource_type":
			return a.ResourceType, a.ResourceType != ""
		case "port":
			return float64(e.Port), e.Port != 0
		case "protocol":
			return e.Protocol, e.Protocol != ""
		case "service":
			return derefString(e.Service)
		case "version":
			return derefString(e.Version)
		case "compliance_status":
			return derefString(e.ComplianceStatus)
		case "allowlist_status":
			return derefString(e.AllowlistStatus)
		case "first_seen":
			return e.FirstSeen.Format(time.RFC3339), true
		case "last_seen":
			return e.LastSeen.Format(time.RFC3339), true
		}
		if strings.HasPrefix(path, "technologies.") {
			tag := strings.TrimPrefix(path, "technologies.")
			matches := technologiesContains(e.Technologies, tag)
			return matches, len(matches) > 0
		}
		return nil, false
	}
}

func findingLookup(f *model.Finding) fieldLookup {
	return func(path string) (any, bool) {
		switch path {
		case "severity":
			return derefString(f.Severity)
		case "source_kind":
			return f.SourceKind, f.SourceKind != ""
		case "source":
			return f.Source, f.Source != ""
		case "source_id":
			return derefString(f.SourceID)
		case "cve_id":
			return derefString(f.CVEID)
		case "status":
			return f.Status, f.Status != ""
		case "title":
			return f.Title, f.Title != ""
		case "asset_endpoint_id":
			return f.AssetEndpointID, f.AssetEndpointID != ""
		case "first_seen":
			return f.FirstSeen.Format(time.RFC3339), true
		case "last_seen":
			return f.LastSeen.Format(time.RFC3339), true
		case "resolved_at":
			if f.ResolvedAt == nil {
				return nil, false
			}
			return f.ResolvedAt.Format(time.RFC3339), true
		}
		return nil, false
	}
}

// --- scalar + operator primitives (straight port) ------------------

func derefString(p *string) (any, bool) {
	if p == nil || *p == "" {
		return nil, false
	}
	return *p, true
}

func scalarEq(want, got any) bool {
	if wn, ok := want.(float64); ok {
		if gn, ok := got.(float64); ok {
			return wn == gn
		}
	}
	if wb, ok := want.(bool); ok {
		if gb, ok := got.(bool); ok {
			return wb == gb
		}
	}
	if ws, ok := want.(string); ok {
		if arr, ok := got.([]string); ok {
			for _, s := range arr {
				if s == ws {
					return true
				}
			}
			return false
		}
		if gs, ok := got.(string); ok {
			return ws == gs
		}
	}
	return false
}

func anyScalarEq(arr []any, got any) bool {
	for _, w := range arr {
		if scalarEq(w, got) {
			return true
		}
	}
	return false
}

func cidrContains(cidr string, got any) bool {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	s, _ := got.(string)
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	return n.Contains(ip)
}

func regexMatch(pattern string, got any) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	switch g := got.(type) {
	case string:
		return re.MatchString(g)
	case []string:
		for _, s := range g {
			if re.MatchString(s) {
				return true
			}
		}
	}
	return false
}

func numCompare(op string, want, got any) bool {
	wn, ok1 := want.(float64)
	gn, ok2 := got.(float64)
	if !ok1 || !ok2 {
		ws, _ := want.(string)
		gs, _ := got.(string)
		switch op {
		case "$gt":
			return gs > ws
		case "$gte":
			return gs >= ws
		case "$lt":
			return gs < ws
		case "$lte":
			return gs <= ws
		}
		return false
	}
	switch op {
	case "$gt":
		return gn > wn
	case "$gte":
		return gn >= wn
	case "$lt":
		return gn < wn
	case "$lte":
		return gn <= wn
	}
	return false
}

func technologiesContains(raw json.RawMessage, want string) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		out := []string{}
		for _, s := range arr {
			if strings.EqualFold(s, want) {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
