package handler

import "net/http"

// DashboardHandler serves KPI aggregates + Suggested Actions
// computation for the new Dashboard. Full impl lands in P5. P1 returns
// 501 so the frontend can feature-flag the page.
type DashboardHandler struct{}

func NewDashboardHandler(_ any) *DashboardHandler { return &DashboardHandler{} }

func (h *DashboardHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "dashboard summary lands in P5")
}
