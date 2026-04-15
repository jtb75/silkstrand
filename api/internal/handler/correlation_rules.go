package handler

import (
	"net/http"
)

// CorrelationRulesHandler serves the new ADR 006 D6 rule shape where
// rule bodies reference a Collection by id instead of inlining a
// predicate. P1 stubs all handlers with 501 — full CRUD wires in P2
// alongside the rule engine's collection-aware dispatcher.
type CorrelationRulesHandler struct{}

func NewCorrelationRulesHandler(_ any) *CorrelationRulesHandler {
	return &CorrelationRulesHandler{}
}

// CreateRuleRequest is the new request shape (ADR 006 D6):
//
//	{
//	  "name": "auto-scan-new-postgres",
//	  "trigger": "asset_discovered",
//	  "body": {
//	    "match":   { "collection_id": "<uuid>" },
//	    "actions": [ ... ]
//	  }
//	}
type CreateRuleRequest struct {
	Name    string         `json:"name"`
	Trigger string         `json:"trigger"`
	Enabled *bool          `json:"enabled,omitempty"`
	Body    map[string]any `json:"body"`
}

func (h *CorrelationRulesHandler) List(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "correlation_rules list lands in P2 with collection-aware rule engine")
}

func (h *CorrelationRulesHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "correlation_rules get lands in P2")
}

func (h *CorrelationRulesHandler) Create(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "correlation_rules create lands in P2 (body.match → {collection_id})")
}

func (h *CorrelationRulesHandler) Update(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "correlation_rules update lands in P2")
}

func (h *CorrelationRulesHandler) Delete(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "correlation_rules delete lands in P2")
}
