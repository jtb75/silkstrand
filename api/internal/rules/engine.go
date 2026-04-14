package rules

import (
	"encoding/json"
	"log/slog"

	"github.com/jtb75/silkstrand/api/internal/model"
)

// RuleBody is the parsed JSONB body of a correlation_rules row.
//
// Example shape:
//
//	{
//	  "match":   { "service": "postgresql", "version": { "$regex": "^16\\." } },
//	  "actions": [
//	     { "type": "suggest_target",     "bundle_id": "<uuid-or-name>" },
//	     { "type": "auto_create_target", "bundle_id": "<uuid-or-name>" }
//	  ]
//	}
type RuleBody struct {
	Match   json.RawMessage          `json:"match"`
	Actions []map[string]interface{} `json:"actions"`
}

// Action types (R1b ships the first two; the rest land in R1c).
const (
	ActionSuggestTarget    = "suggest_target"
	ActionAutoCreateTarget = "auto_create_target"
	ActionNotify           = "notify"           // R1c
	ActionRunOneShotScan   = "run_one_shot_scan"// R1c
)

// EvaluateAsset runs every rule whose trigger=='asset_discovered'
// against the asset and returns the actions that fired. Rule errors
// are logged but don't stop the pass — a malformed rule must not
// break the ingest path.
func EvaluateAsset(rules []model.CorrelationRule, asset *model.DiscoveredAsset) []FiredAction {
	var out []FiredAction
	for _, r := range rules {
		if !r.Enabled || r.Trigger != model.RuleTriggerAssetDiscovered {
			continue
		}
		var body RuleBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			slog.Warn("rule body unmarshal", "rule", r.Name, "tenant", r.TenantID, "error", err)
			continue
		}
		match, err := Match(body.Match, asset)
		if err != nil {
			slog.Warn("rule match error", "rule", r.Name, "tenant", r.TenantID, "error", err)
			continue
		}
		if !match {
			continue
		}
		for _, a := range body.Actions {
			t, _ := a["type"].(string)
			out = append(out, FiredAction{
				RuleID:   r.ID,
				RuleName: r.Name,
				Type:     t,
				Params:   a,
			})
		}
	}
	return out
}

// FiredAction describes a single matched-rule action ready for the
// caller to execute. Caller switches on Type to perform the side
// effect (create target, post notification, etc.).
type FiredAction struct {
	RuleID   string
	RuleName string
	Type     string
	Params   map[string]interface{}
}

// BundleID extracts the canonical bundle reference from the action
// params. Accepts either bundle_id or bundle (string) for ergonomics
// in the rule YAML/JSON.
func (a FiredAction) BundleID() string {
	if v, ok := a.Params["bundle_id"].(string); ok && v != "" {
		return v
	}
	if v, ok := a.Params["bundle"].(string); ok && v != "" {
		return v
	}
	return ""
}
