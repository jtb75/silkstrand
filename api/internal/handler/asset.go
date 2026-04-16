package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/rules"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// AssetHandler serves the tenant Assets page. P4 adds coverage + risk
// roll-ups to List and Get, and ships the endpoint detail route.
type AssetHandler struct {
	store store.Store
}

func NewAssetHandler(s store.Store) *AssetHandler {
	return &AssetHandler{store: s}
}

// Coverage is the per-asset roll-up served alongside each Asset. See
// docs/plans/ui-shape.md § Asset coverage.
type Coverage struct {
	EndpointsTotal                int       `json:"endpoints_total"`
	EndpointsWithScanDefinition   int       `json:"endpoints_with_scan_definition"`
	EndpointsWithCredentialMapping int      `json:"endpoints_with_credential_mapping"`
	LastScanAt                    *time.Time `json:"last_scan_at,omitempty"`
	NextScanAt                    *time.Time `json:"next_scan_at,omitempty"`
	Gaps                          []string   `json:"gaps"`
}

// Risk is the per-asset open-findings severity rollup.
type Risk struct {
	Critical           int `json:"critical"`
	High               int `json:"high"`
	Medium             int `json:"medium"`
	Low                int `json:"low"`
	Info               int `json:"info"`
	TrendVsPrevious    int `json:"trend_vs_previous"` // placeholder: 0 until ADR 007+ ships trend window
}

// GET /api/v1/assets — list with coverage + risk.
func (h *AssetHandler) List(w http.ResponseWriter, r *http.Request) {
	f := store.AssetFilter{
		Source: r.URL.Query().Get("source"),
	}
	items, total, err := h.store.ListAssets(r.Context(), f)
	if err != nil {
		slog.Error("listing assets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}
	if items == nil {
		items = []model.Asset{}
	}
	rolls, err := buildRollups(r.Context(), h.store)
	if err != nil {
		slog.Error("rollup: load", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load coverage")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		cov, risk := rolls.forAsset(&items[i])
		flat := flattenAsset(&items[i], cov, risk)
		out = append(out, flat)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     out,
		"page":      1,
		"page_size": len(out),
		"total":     total,
	})
}

// GET /api/v1/assets/{id} — detail with events, endpoints, coverage + risk.
func (h *AssetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := h.store.GetAssetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}
	endpoints, err := h.store.ListEndpointsForAssetTenant(r.Context(), id)
	if err != nil {
		slog.Error("load endpoints", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load endpoints")
		return
	}
	rolls, err := buildRollups(r.Context(), h.store)
	if err != nil {
		slog.Error("rollup: load", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load coverage")
		return
	}
	cov, risk := rolls.forAsset(a)
	flat := flattenAsset(a, cov, risk)
	flat["endpoints"] = endpoints
	flat["events"] = []any{}
	writeJSON(w, http.StatusOK, flat)
}

// GET /api/v1/assets/{id}/endpoints/{endpoint_id}
func (h *AssetHandler) GetEndpoint(w http.ResponseWriter, r *http.Request) {
	assetID := r.PathValue("id")
	endpointID := r.PathValue("endpoint_id")
	e, a, err := h.store.GetAssetEndpointByID(r.Context(), endpointID)
	if err != nil {
		slog.Error("get endpoint", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if e == nil || a == nil || a.ID != assetID {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}
	findings, err := h.store.ListFindingsForEndpoint(r.Context(), endpointID)
	if err != nil {
		slog.Error("list findings", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load findings")
		return
	}
	// Credential binding: does any credential-mapped collection
	// (endpoint-scope) match this endpoint?
	mappedColls, err := h.store.CollectionsWithCredentialMappings(r.Context())
	if err != nil {
		slog.Error("load mapped collections", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	bound := credentialBoundForEndpoint(a, e, mappedColls)

	// Coverage status per-endpoint: has scan_definition? has credential
	// binding? is it allowlisted?
	rolls, err := buildRollups(r.Context(), h.store)
	if err != nil {
		slog.Error("rollup: load", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	_, hasSD := rolls.sdEndpoints[a.ID][e.ID]
	status := map[string]any{
		"has_scan_definition":       hasSD,
		"has_credential_mapping":    bound,
		"allowlist_status":          e.AllowlistStatus,
		"missed_scan_count":         e.MissedScanCount,
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"asset":              a,
		"endpoint":           e,
		"findings":           findings,
		"credential_binding": map[string]any{"matched": bound},
		"coverage_status":    status,
	})
}

// Promote — removed (see P4 brief); kept for route compatibility.
func (h *AssetHandler) Promote(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented,
		"asset promote is superseded by scan_definitions (scope=asset_endpoint); P3")
}

// flattenAsset spreads the model.Asset fields at the top level and adds
// coverage + risk as sibling objects alongside an endpoints_count
// convenience field so the UI can render columns without nested access.
func flattenAsset(a *model.Asset, cov Coverage, risk Risk) map[string]any {
	return map[string]any{
		"id":              a.ID,
		"tenant_id":       a.TenantID,
		"primary_ip":      a.PrimaryIP,
		"hostname":        a.Hostname,
		"fingerprint":     a.Fingerprint,
		"resource_type":   a.ResourceType,
		"source":          a.Source,
		"environment":     a.Environment,
		"first_seen":      a.FirstSeen,
		"last_seen":       a.LastSeen,
		"created_at":      a.CreatedAt,
		"endpoints_count": cov.EndpointsTotal,
		"coverage":        cov,
		"risk":            risk,
	}
}

// ---------------------------------------------------------------- rollups

type rollups struct {
	endpointsByAsset map[string][]*model.AssetEndpoint
	sdEndpoints      map[string]map[string]struct{}
	lastScan         map[string]time.Time
	nextScan         map[string]time.Time
	sevByEndpoint    map[string]map[string]int
	credMatchByEp    map[string]bool // endpointID → matches any credential-mapped collection
}

func buildRollups(ctx context.Context, s store.Store) (*rollups, error) {
	views, err := s.ListAllEndpointViewsTenant(ctx)
	if err != nil {
		return nil, err
	}
	epByAsset := map[string][]*model.AssetEndpoint{}
	for i := range views {
		v := views[i] // stable copy
		epByAsset[v.Asset.ID] = append(epByAsset[v.Asset.ID], &v.Endpoint)
	}
	sd, err := s.EndpointsWithScanDefinitionByAsset(ctx)
	if err != nil {
		return nil, err
	}
	ls, err := s.LastScanAtByAsset(ctx)
	if err != nil {
		return nil, err
	}
	ns, err := s.NextScanAtByAsset(ctx)
	if err != nil {
		return nil, err
	}
	sev, err := s.FindingsSeverityByEndpoint(ctx)
	if err != nil {
		return nil, err
	}
	mappedColls, err := s.CollectionsWithCredentialMappings(ctx)
	if err != nil {
		return nil, err
	}
	credMatch := map[string]bool{}
	if len(mappedColls) > 0 {
		for i := range views {
			v := views[i]
			for _, c := range mappedColls {
				if matchesCollection(&v.Asset, &v.Endpoint, c) {
					credMatch[v.Endpoint.ID] = true
					break
				}
			}
		}
	}
	return &rollups{
		endpointsByAsset: epByAsset,
		sdEndpoints:      sd,
		lastScan:         ls,
		nextScan:         ns,
		sevByEndpoint:    sev,
		credMatchByEp:    credMatch,
	}, nil
}

func (r *rollups) forAsset(a *model.Asset) (Coverage, Risk) {
	cov := Coverage{Gaps: []string{}}
	eps := r.endpointsByAsset[a.ID]
	cov.EndpointsTotal = len(eps)
	sd := r.sdEndpoints[a.ID]
	cov.EndpointsWithScanDefinition = len(sd)
	for _, e := range eps {
		if r.credMatchByEp[e.ID] {
			cov.EndpointsWithCredentialMapping++
		}
	}
	if t, ok := r.lastScan[a.ID]; ok {
		tt := t
		cov.LastScanAt = &tt
	}
	if t, ok := r.nextScan[a.ID]; ok {
		tt := t
		cov.NextScanAt = &tt
	}
	if cov.EndpointsTotal == 0 {
		cov.Gaps = append(cov.Gaps, "no_endpoints")
	} else {
		if cov.EndpointsWithScanDefinition < cov.EndpointsTotal {
			cov.Gaps = append(cov.Gaps, "missing_scan_definition")
		}
		if cov.EndpointsWithCredentialMapping < cov.EndpointsTotal {
			cov.Gaps = append(cov.Gaps, "missing_credential_mapping")
		}
	}

	var risk Risk
	for _, e := range eps {
		sev := r.sevByEndpoint[e.ID]
		risk.Critical += sev["critical"]
		risk.High += sev["high"]
		risk.Medium += sev["medium"]
		risk.Low += sev["low"]
		risk.Info += sev["info"]
	}
	return cov, risk
}

// matchesCollection evaluates an endpoint-scope (or asset-scope)
// collection predicate against an (asset, endpoint) pair.
func matchesCollection(a *model.Asset, e *model.AssetEndpoint, c model.Collection) bool {
	switch c.Scope {
	case model.CollectionScopeEndpoint:
		ok, err := rules.Match(c.Predicate, rules.ScopeEndpoint, rules.EndpointView{Asset: a, Endpoint: e})
		return err == nil && ok
	case model.CollectionScopeAsset:
		ok, err := rules.Match(c.Predicate, rules.ScopeAsset, a)
		return err == nil && ok
	}
	return false
}

// credentialBoundForEndpoint is the single-endpoint variant used by the
// endpoint-detail handler.
func credentialBoundForEndpoint(a *model.Asset, e *model.AssetEndpoint, colls []model.Collection) bool {
	for _, c := range colls {
		if matchesCollection(a, e, c) {
			return true
		}
	}
	return false
}

// guard: keep json import live when model.Asset etc. are the only thing
// we serialize directly (writeJSON imports json transitively).
var _ = json.RawMessage{}
