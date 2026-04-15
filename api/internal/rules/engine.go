// Package rules hosts the D2 rule engine. Post-ADR-006 D6 rule bodies
// reference a collection by id rather than carrying an inline predicate.
// The engine is stubbed in P1 — see predicate.go. P2 will re-introduce
// EvaluateAsset / EvaluateFinding call sites and wire them into ingest.
package rules

import (
	"encoding/json"
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

// Action types. The dispatcher wiring (side effects) lives in the
// ingest package; engine just classifies which action fired.
const (
	ActionSuggestTarget    = "suggest_target"
	ActionAutoCreateTarget = "auto_create_target"
	ActionNotify           = "notify"
	ActionRunOneShotScan   = "run_one_shot_scan"
)

// FiredAction describes a single matched-rule action ready for the
// caller to execute. Identical shape to pre-refactor for ease of
// migration in P2.
type FiredAction struct {
	RuleID   string
	RuleName string
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
