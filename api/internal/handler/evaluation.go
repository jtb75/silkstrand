package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/audit"
	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/policy"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// EvaluationHandler provides retroactive policy evaluation — replaying
// stored facts against the current policy set (ADR 011 D10).
type EvaluationHandler struct {
	store     store.Store
	evaluator *policy.Evaluator
	audit     audit.Writer
}

func NewEvaluationHandler(s store.Store, ev *policy.Evaluator, aw audit.Writer) *EvaluationHandler {
	return &EvaluationHandler{store: s, evaluator: ev, audit: aw}
}

type replayRequest struct {
	ScanID     string   `json:"scan_id"`
	EndpointID string   `json:"endpoint_id"`
	PolicyIDs  []string `json:"policy_ids"`
	DryRun     bool     `json:"dry_run"`
}

type replayResponse struct {
	Results         []policy.Result `json:"results"`
	DryRun          bool            `json:"dry_run"`
	FindingsCreated int             `json:"findings_created"`
	FindingsUpdated int             `json:"findings_updated"`
}

// Replay re-evaluates stored collected facts against the current policy
// set. Exactly one of scan_id or endpoint_id must be provided.
func (h *EvaluationHandler) Replay(w http.ResponseWriter, r *http.Request) {
	var req replayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if (req.ScanID == "") == (req.EndpointID == "") {
		writeError(w, http.StatusBadRequest, "exactly one of scan_id or endpoint_id must be provided")
		return
	}

	ctx := r.Context()
	tenantID := store.TenantID(ctx)

	// Collect all facts rows to evaluate.
	type factsRow struct {
		AssetEndpointID string
		ScanID          string
		Facts           map[string]any
	}
	var rows []factsRow

	if req.ScanID != "" {
		collected, err := h.store.GetCollectedFactsByScan(ctx, req.ScanID)
		if err != nil {
			slog.Error("loading facts by scan", "error", err, "scan_id", req.ScanID)
			writeError(w, http.StatusInternalServerError, "failed to load facts")
			return
		}
		if len(collected) == 0 {
			writeError(w, http.StatusNotFound, "no collected facts for scan_id")
			return
		}
		for _, c := range collected {
			var facts map[string]any
			if err := json.Unmarshal(c.Facts, &facts); err != nil {
				slog.Warn("skipping unparseable facts row", "id", c.ID, "error", err)
				continue
			}
			rows = append(rows, factsRow{
				AssetEndpointID: c.AssetEndpointID,
				ScanID:          c.ScanID,
				Facts:           facts,
			})
		}
	} else {
		latest, err := h.store.GetLatestFactsForEndpoint(ctx, req.EndpointID)
		if err != nil {
			slog.Error("loading latest facts for endpoint", "error", err, "endpoint_id", req.EndpointID)
			writeError(w, http.StatusInternalServerError, "failed to load facts")
			return
		}
		if latest == nil {
			writeError(w, http.StatusNotFound, "no collected facts for endpoint_id")
			return
		}
		var facts map[string]any
		if err := json.Unmarshal(latest.Facts, &facts); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse stored facts")
			return
		}
		rows = append(rows, factsRow{
			AssetEndpointID: latest.AssetEndpointID,
			ScanID:          latest.ScanID,
			Facts:           facts,
		})
	}

	// Build a policy_id filter set if the caller asked for specific policies.
	policyFilter := make(map[string]struct{}, len(req.PolicyIDs))
	for _, pid := range req.PolicyIDs {
		policyFilter[pid] = struct{}{}
	}

	var allResults []policy.Result
	var created, updated int

	for _, fr := range rows {
		results := h.evaluator.Evaluate(ctx, fr.Facts)

		// Filter to requested policy IDs if specified.
		if len(policyFilter) > 0 {
			filtered := make([]policy.Result, 0, len(results))
			for _, res := range results {
				if _, ok := policyFilter[res.ControlID]; ok {
					filtered = append(filtered, res)
				}
			}
			results = filtered
		}

		allResults = append(allResults, results...)

		if !req.DryRun {
			for _, res := range results {
				status := "open"
				if res.Status == "pass" {
					status = "resolved"
				}
				evidence, _ := json.Marshal(res.Evidence)
				scanID := &fr.ScanID

				existing, err := h.store.UpsertFinding(ctx, store.UpsertFindingInput{
					TenantID:        tenantID,
					AssetEndpointID: fr.AssetEndpointID,
					ScanID:          scanID,
					SourceKind:      "bundle_compliance",
					Source:          res.ControlID,
					SourceID:        res.ControlID,
					Severity:        strPtr(res.Severity),
					Title:           res.Title,
					Status:          status,
					Evidence:        evidence,
					Remediation:     strPtr(res.Remediation),
				})
				if err != nil {
					slog.Error("upserting finding during replay", "error", err, "control_id", res.ControlID)
					continue
				}
				// If the finding ID was just created (first_seen == last_seen), count as created.
				if existing != nil && existing.FirstSeen.Equal(existing.LastSeen) {
					created++
				} else {
					updated++
				}
			}
		}
	}

	// Audit event
	resourceID := req.ScanID
	if resourceID == "" {
		resourceID = req.EndpointID
	}
	claims := middleware.GetClaims(ctx)
	h.audit.Emit(ctx, audit.Event{
		TenantID:     tenantID,
		EventType:    "evaluation.replayed",
		ActorType:    "user",
		ActorID:      claimsActorID(claims),
		ResourceType: "evaluation",
		ResourceID:   resourceID,
		Payload: map[string]any{
			"scan_id":          req.ScanID,
			"endpoint_id":      req.EndpointID,
			"policy_count":     len(allResults),
			"findings_created": created,
			"findings_updated": updated,
			"dry_run":          req.DryRun,
		},
	})

	writeJSON(w, http.StatusOK, replayResponse{
		Results:         allResults,
		DryRun:          req.DryRun,
		FindingsCreated: created,
		FindingsUpdated: updated,
	})
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
