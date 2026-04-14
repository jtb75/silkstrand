// Package rules implements the ADR 003 D2 generalized match → action
// engine. Predicates are JSONB documents stored in correlation_rules.body
// and asset_sets.predicate. The matcher walks the document and checks
// each term against a model.DiscoveredAsset.
package rules

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
)

// Match evaluates a predicate JSONB against an asset and returns true on
// a match. Invalid predicates return (false, error) — the caller decides
// whether to skip the rule or treat it as no-match.
func Match(predicate json.RawMessage, asset *model.DiscoveredAsset) (bool, error) {
	if len(predicate) == 0 {
		return true, nil
	}
	var node any
	if err := json.Unmarshal(predicate, &node); err != nil {
		return false, fmt.Errorf("invalid predicate: %w", err)
	}
	return evalNode(node, asset)
}

func evalNode(node any, a *model.DiscoveredAsset) (bool, error) {
	obj, ok := node.(map[string]any)
	if !ok {
		return false, fmt.Errorf("predicate node must be object, got %T", node)
	}
	// Compound: $and / $or / $not at the top of the object.
	if v, ok := obj["$and"]; ok {
		return evalAnd(v, a)
	}
	if v, ok := obj["$or"]; ok {
		return evalOr(v, a)
	}
	if v, ok := obj["$not"]; ok {
		ok, err := evalNode(v, a)
		return !ok, err
	}
	// Term: each key is a field path, value is scalar or operator object.
	for field, val := range obj {
		ok, err := evalTerm(field, val, a)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalAnd(node any, a *model.DiscoveredAsset) (bool, error) {
	arr, ok := node.([]any)
	if !ok {
		return false, fmt.Errorf("$and expects array")
	}
	for _, child := range arr {
		ok, err := evalNode(child, a)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalOr(node any, a *model.DiscoveredAsset) (bool, error) {
	arr, ok := node.([]any)
	if !ok {
		return false, fmt.Errorf("$or expects array")
	}
	for _, child := range arr {
		ok, err := evalNode(child, a)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func evalTerm(field string, val any, a *model.DiscoveredAsset) (bool, error) {
	// Operator object: { $eq: ..., $in: [...], $cidr: ..., ... }
	if op, ok := val.(map[string]any); ok && hasOperatorKey(op) {
		return evalOperator(field, op, a)
	}
	// Bare scalar: implicit $eq.
	got, present := assetField(field, a)
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

func evalOperator(field string, op map[string]any, a *model.DiscoveredAsset) (bool, error) {
	got, present := assetField(field, a)
	for k, v := range op {
		switch k {
		case "$exists":
			want, _ := v.(bool)
			if (present) != want {
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

// assetField resolves a dotted path into the asset model. Returns
// (value, present). Returns (nil, false) for missing or empty fields.
func assetField(path string, a *model.DiscoveredAsset) (any, bool) {
	switch path {
	case "ip":
		return a.IP, a.IP != ""
	case "port":
		return float64(a.Port), a.Port != 0
	case "hostname":
		return derefStringField(a.Hostname)
	case "service":
		return derefStringField(a.Service)
	case "version":
		return derefStringField(a.Version)
	case "environment":
		return derefStringField(a.Environment)
	case "source":
		return a.Source, a.Source != ""
	case "compliance_status":
		return derefStringField(a.ComplianceStatus)
	case "first_seen":
		return a.FirstSeen.Format(time.RFC3339), true
	case "last_seen":
		return a.LastSeen.Format(time.RFC3339), true
	}
	// Composite paths. Presence tracks whether the lookup produced any
	// values — empty arrays ≡ not present so $exists:false matches the
	// obvious case.
	if strings.HasPrefix(path, "technologies.") {
		tag := strings.TrimPrefix(path, "technologies.")
		matches := technologiesContains(a.Technologies, tag)
		return matches, len(matches) > 0
	}
	if path == "cves" {
		ids := cvesIDs(a.CVEs)
		return ids, len(ids) > 0
	}
	if strings.HasPrefix(path, "cves.") {
		key := strings.TrimPrefix(path, "cves.")
		switch key {
		case "severity":
			sev := cvesSeverities(a.CVEs)
			return sev, len(sev) > 0
		case "id":
			ids := cvesIDs(a.CVEs)
			return ids, len(ids) > 0
		}
	}
	return nil, false
}

func derefStringField(p *string) (any, bool) {
	if p == nil || *p == "" {
		return nil, false
	}
	return *p, true
}

func scalarEq(want, got any) bool {
	// Treat numbers liberally — JSON unmarshals numbers as float64.
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
		// String wanted but got a list (technologies/cves array): substring match.
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
		// Handle string-comparison for time fields too.
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

func cvesIDs(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	out := []string{}
	for _, item := range arr {
		if s, ok := item["id"].(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func cvesSeverities(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	out := []string{}
	for _, item := range arr {
		if s, ok := item["severity"].(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
