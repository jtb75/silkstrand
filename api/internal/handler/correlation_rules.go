package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/rules"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// CorrelationRulesHandler serves CRUD for the ADR 003 D2 rule engine.
// R1b ships with two action types (suggest_target, auto_create_target).
type CorrelationRulesHandler struct {
	store store.Store
}

func NewCorrelationRulesHandler(s store.Store) *CorrelationRulesHandler {
	return &CorrelationRulesHandler{store: s}
}

// GET /api/v1/correlation-rules — every rule for this tenant
// (latest + historical versions; UI shows latest by default).
func (h *CorrelationRulesHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListAllRules(r.Context())
	if err != nil {
		slog.Error("listing rules", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if items == nil {
		items = []model.CorrelationRule{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /api/v1/correlation-rules/{id}
func (h *CorrelationRulesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rule, err := h.store.GetRule(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if rule == nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// POST /api/v1/correlation-rules
// Body: { name, trigger, enabled?, body: {match, actions[]} }
// Versioning is automatic — every PUT/POST writes a new version with
// the same name.
func (h *CorrelationRulesHandler) Create(w http.ResponseWriter, r *http.Request) {
	rule, err := h.parseRule(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := h.store.UpsertRule(r.Context(), *rule)
	if err != nil {
		slog.Error("creating rule", "error", err, "name", rule.Name)
		writeError(w, http.StatusInternalServerError, "failed to create rule")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// PUT /api/v1/correlation-rules/{id} — bumps version on the rule's
// (tenant_id, name); the path id is informational (used only to
// preserve the rule name on the wire).
func (h *CorrelationRulesHandler) Update(w http.ResponseWriter, r *http.Request) {
	existing, err := h.store.GetRule(r.Context(), r.PathValue("id"))
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	rule, err := h.parseRule(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rule.Name = existing.Name // freeze name so a PUT can't rename
	out, err := h.store.UpsertRule(r.Context(), *rule)
	if err != nil {
		slog.Error("updating rule", "error", err, "name", rule.Name)
		writeError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// DELETE /api/v1/correlation-rules/{id}
func (h *CorrelationRulesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteRule(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CorrelationRulesHandler) parseRule(r *http.Request) (*model.CorrelationRule, error) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		return nil, errors.New("unauthorized")
	}
	var req struct {
		Name            string          `json:"name"`
		Trigger         string          `json:"trigger"`
		Enabled         *bool           `json:"enabled"`
		EventTypeFilter *string         `json:"event_type_filter"`
		Body            json.RawMessage `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.New("invalid request body")
	}
	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if req.Trigger != model.RuleTriggerAssetDiscovered && req.Trigger != model.RuleTriggerAssetEvent {
		return nil, errors.New("trigger must be asset_discovered or asset_event")
	}
	// Validate body shape: must parse as RuleBody and contain at least
	// one supported action type. Reject unknown actions early.
	var body rules.RuleBody
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return nil, errors.New("body must be a JSON object with match + actions[]")
	}
	if len(body.Actions) == 0 {
		return nil, errors.New("body.actions must contain at least one action")
	}
	for _, a := range body.Actions {
		t, _ := a["type"].(string)
		switch t {
		case rules.ActionSuggestTarget, rules.ActionAutoCreateTarget,
			rules.ActionNotify, rules.ActionRunOneShotScan:
		default:
			return nil, errors.New("unknown action type: " + t)
		}
		if t == rules.ActionNotify {
			if _, ok := a["channel"].(string); !ok {
				return nil, errors.New("notify action requires a 'channel' string")
			}
		}
		if t == rules.ActionRunOneShotScan {
			if _, ok := a["bundle_id"].(string); !ok {
				if _, ok := a["bundle"].(string); !ok {
					return nil, errors.New("run_one_shot_scan action requires bundle_id")
				}
			}
			if _, ok := a["agent_id"].(string); !ok {
				return nil, errors.New("run_one_shot_scan action requires agent_id")
			}
		}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	createdBy := claims.Email
	return &model.CorrelationRule{
		TenantID:        claims.TenantID,
		Name:            req.Name,
		Enabled:         enabled,
		Trigger:         req.Trigger,
		EventTypeFilter: req.EventTypeFilter,
		Body:            req.Body,
		CreatedBy:       &createdBy,
	}, nil
}
