package handler

import "net/http"

// CredentialMappingsHandler serves the collection ↔ credential_source
// bindings (ADR 006 roadmap P6). P5 ships the full impl including
// bulk-apply + the tenant-facing Credentials page. P1 returns 501.
type CredentialMappingsHandler struct{}

func NewCredentialMappingsHandler(_ any) *CredentialMappingsHandler {
	return &CredentialMappingsHandler{}
}

func (h *CredentialMappingsHandler) List(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "credential_mappings list lands in P5")
}

func (h *CredentialMappingsHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "credential_mappings get lands in P5")
}

func (h *CredentialMappingsHandler) Create(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "credential_mappings create lands in P5")
}

func (h *CredentialMappingsHandler) Delete(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "credential_mappings delete lands in P5")
}
