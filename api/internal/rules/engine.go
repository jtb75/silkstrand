// Package rules hosts the D2 rule engine. Post-ADR-006 D6 rule bodies
// reference a collection by id rather than carrying an inline predicate.
// EvaluateAsset loads each rule's referenced collection, dispatches on
// scope, and returns the actions that fired.
package rules

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jtb75/silkstrand/api/internal/model"
)

// RuleBody is the parsed JSONB body of a correlation_rules row. Post
// ADR 006 D6 `match` is an object containing `collection_id` rather than
// an inline predicate document.
type RuleBody struct {
	Match   json.RawMessage          `json:"match"`
	Actions []map[string]interface{} `json:"actions"`
}

// MatchRef is the shape of RuleBody.Match post-ADR-006 D6.
type MatchRef struct {
	CollectionID string `json:"collection_id"`
}

// Action types.
const (
	ActionSuggestTarget    = "suggest_target"
	ActionAutoCreateTarget = "auto_create_target"
	ActionNotify           = "notify"
	// ActionRunScanDefinition is the new name; ActionRunOneShotScan is
	// accepted as a deprecated alias so older seed files keep parsing.
	ActionRunScanDefinition = "run_scan_definition"
	ActionRunOneShotScan    = "run_one_shot_scan"
)

// CanonicalActionKind normalizes deprecated action names to their
// current form and logs a one-shot deprecation warning.
func CanonicalActionKind(kind string) string {
	switch kind {
	case ActionRunOneShotScan:
		slog.Warn("rule.action.deprecated_kind",
			"old", ActionRunOneShotScan, "new", ActionRunScanDefinition)
		return ActionRunScanDefinition
	}
	return kind
}

// FiredAction describes a single matched-rule action ready for the
// caller to execute. Identical shape to pre-refactor for ease of
// migration.
type FiredAction struct {
	RuleID   string
	RuleName string
	TenantID string
	Type     string
	Params   map[string]interface{}
}

// BundleID extracts the canonical bundle reference from the action
// params. Accepts either bundle_id or bundle (string) for ergonomics.
func (a FiredAction) BundleID() string {
	if v, ok := a.Params["bundle_id"].(string); ok && v != "" {
		return v
	}
	if v, ok := a.Params["bundle"].(string); ok && v != "" {
		return v
	}
	return ""
}

// CollectionLoader resolves a collection_id to its stored predicate +
// scope. The engine is deliberately decoupled from the store interface
// so tests can inject fakes.
type CollectionLoader interface {
	GetCollection(ctx context.Context, id string) (*model.Collection, error)
}

// EvaluateAsset walks the rule set and returns the actions that fired
// on the given (asset, endpoint) pair. Rules are skipped (with a log)
// on any of:
//
//   - disabled
//   - body.match missing or malformed
//   - referenced collection missing
//   - rule trigger incompatible with the collection's scope
//   - predicate evaluation error
//
// Rule-load errors never break ingest — the caller keeps upserting.
func EvaluateAsset(
	ctx context.Context,
	loader CollectionLoader,
	rules []model.CorrelationRule,
	asset *model.Asset,
	endpoint *model.AssetEndpoint,
) []FiredAction {
	if asset == nil || endpoint == nil {
		return nil
	}
	view := EndpointView{Asset: asset, Endpoint: endpoint}
	var out []FiredAction

	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		// P2 fires asset_discovered / asset_event rules on discovery.
		// finding-triggered rules wait for P3 findings ingest.
		if r.Trigger == model.RuleTriggerFinding {
			continue
		}

		var body RuleBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			slog.Warn("rule.body_unmarshal",
				"rule", r.Name, "tenant", r.TenantID, "error", err)
			continue
		}
		var ref MatchRef
		if err := json.Unmarshal(body.Match, &ref); err != nil || ref.CollectionID == "" {
			slog.Warn("rule.match_ref_invalid",
				"rule", r.Name, "tenant", r.TenantID, "error", err)
			continue
		}
		coll, err := loader.GetCollection(ctx, ref.CollectionID)
		if err != nil {
			slog.Warn("rule.collection_load",
				"rule", r.Name, "collection", ref.CollectionID, "error", err)
			continue
		}
		if coll == nil {
			slog.Warn("rule.collection_missing",
				"rule", r.Name, "collection", ref.CollectionID)
			continue
		}
		if !triggerMatchesScope(r.Trigger, coll.Scope) {
			slog.Warn("rule.trigger_scope_mismatch",
				"rule", r.Name, "trigger", r.Trigger, "scope", coll.Scope)
			continue
		}

		scope := Scope(coll.Scope)
		var subject any
		switch scope {
		case ScopeAsset:
			subject = asset
		case ScopeEndpoint:
			subject = view
		default:
			// finding-scope rules skipped above; any future scope is
			// unknown and gets skipped.
			continue
		}

		matched, err := Match(coll.Predicate, scope, subject)
		if err != nil {
			slog.Warn("rule.match_error",
				"rule", r.Name, "collection", coll.ID, "error", err)
			continue
		}
		if !matched {
			continue
		}
		for _, a := range body.Actions {
			kind, _ := a["type"].(string)
			canonical := CanonicalActionKind(kind)
			out = append(out, FiredAction{
				RuleID:   r.ID,
				RuleName: r.Name,
				TenantID: r.TenantID,
				Type:     canonical,
				Params:   a,
			})
		}
	}
	return out
}

// triggerMatchesScope enforces the ADR 006 D5 compatibility matrix.
// asset_discovered + asset_event fire on asset / endpoint scopes.
// finding triggers require a finding-scope collection (P3 work).
func triggerMatchesScope(trigger, scope string) bool {
	switch trigger {
	case model.RuleTriggerAssetDiscovered, model.RuleTriggerAssetEvent:
		return scope == model.CollectionScopeAsset || scope == model.CollectionScopeEndpoint
	case model.RuleTriggerFinding:
		return scope == model.CollectionScopeFinding
	}
	return false
}
