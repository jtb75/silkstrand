package handler

import "net/http"

// ScanDefinitionsHandler serves ADR 007 D3 scan_definitions CRUD + the
// manual-execute path. Full impl (P3) ships alongside the scheduler
// goroutine. P1 returns 501 on everything.
type ScanDefinitionsHandler struct{}

func NewScanDefinitionsHandler(_ any) *ScanDefinitionsHandler {
	return &ScanDefinitionsHandler{}
}

func (h *ScanDefinitionsHandler) List(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "scan_definitions list lands in P3")
}

func (h *ScanDefinitionsHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "scan_definitions get lands in P3")
}

func (h *ScanDefinitionsHandler) Create(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "scan_definitions create lands in P3")
}

func (h *ScanDefinitionsHandler) Update(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "scan_definitions update lands in P3")
}

func (h *ScanDefinitionsHandler) Delete(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "scan_definitions delete lands in P3")
}

func (h *ScanDefinitionsHandler) Execute(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "scan_definitions execute lands in P3")
}

func (h *ScanDefinitionsHandler) Enable(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "scan_definitions enable lands in P3")
}

func (h *ScanDefinitionsHandler) Disable(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "scan_definitions disable lands in P3")
}
