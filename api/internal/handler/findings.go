package handler

import "net/http"

// FindingsHandler serves the ADR 007 D1 `findings` surface. Full
// implementation lands in P3 (backend: ingest write-through + filter
// surface). P1 returns 501 on all routes.
type FindingsHandler struct{}

func NewFindingsHandler(_ any) *FindingsHandler { return &FindingsHandler{} }

func (h *FindingsHandler) List(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "findings list lands in P3")
}

func (h *FindingsHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "findings get lands in P3")
}

func (h *FindingsHandler) Suppress(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "findings suppress lands in P3")
}

func (h *FindingsHandler) Reopen(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "findings reopen lands in P3")
}
