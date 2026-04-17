package handler

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// maxUploadSize is the maximum allowed bundle tarball size (50 MB).
const maxUploadSize = 50 << 20

type BundlesHandler struct {
	store       store.Store
	storagePath string // local filesystem path for bundle tarballs (v1)
}

func NewBundlesHandler(s store.Store, storagePath string) *BundlesHandler {
	return &BundlesHandler{store: s, storagePath: storagePath}
}

// GET /api/v1/bundles (tenant-authed)
// Returns bundles available to this tenant: global ones + any tenant-owned.
func (h *BundlesHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	bundles, err := h.store.ListBundlesForTenant(r.Context(), claims.TenantID)
	if err != nil {
		slog.Error("listing bundles", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list bundles")
		return
	}
	if bundles == nil {
		bundles = []model.Bundle{}
	}
	writeJSON(w, http.StatusOK, bundles)
}

// GET /api/v1/bundles/{id}/controls (tenant-authed)
// Returns the control metadata for a bundle.
func (h *BundlesHandler) ListControls(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	bundleID := r.PathValue("id")
	if bundleID == "" {
		writeError(w, http.StatusBadRequest, "missing bundle id")
		return
	}

	// Verify bundle exists and is accessible to this tenant.
	bundle, err := h.store.GetBundle(r.Context(), bundleID)
	if err != nil {
		slog.Error("getting bundle", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get bundle")
		return
	}
	if bundle == nil {
		writeError(w, http.StatusNotFound, "bundle not found")
		return
	}
	if bundle.TenantID != nil && *bundle.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "bundle not found")
		return
	}

	controls, err := h.store.ListBundleControls(r.Context(), bundleID)
	if err != nil {
		slog.Error("listing bundle controls", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list controls")
		return
	}
	if controls == nil {
		controls = []model.BundleControl{}
	}
	writeJSON(w, http.StatusOK, controls)
}

// --- Cross-framework control catalog (Level 2A) ---

// controlFrameworkMapping is one bundle that includes a control.
type controlFrameworkMapping struct {
	BundleID   string `json:"bundle_id"`
	BundleName string `json:"bundle_name"`
	Section    string `json:"section"`
}

// controlEntry is one control grouped across all frameworks.
type controlEntry struct {
	ControlID      string                    `json:"control_id"`
	Name           string                    `json:"name"`
	Severity       string                    `json:"severity"`
	Engine         string                    `json:"engine"`
	EngineVersions json.RawMessage           `json:"engine_versions"`
	Tags           json.RawMessage           `json:"tags"`
	Frameworks     []controlFrameworkMapping  `json:"frameworks"`
}

// ListAllControls handles GET /api/v1/controls — cross-framework control catalog.
func (h *BundlesHandler) ListAllControls(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	filter := store.ControlFilter{
		Framework: q.Get("framework"),
		Engine:    q.Get("engine"),
		Severity:  q.Get("severity"),
		Tag:       q.Get("tag"),
		Q:         q.Get("q"),
		Page:      page,
		PageSize:  pageSize,
	}

	rows, total, err := h.store.ListControls(r.Context(), claims.TenantID, filter)
	if err != nil {
		slog.Error("listing controls", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list controls")
		return
	}

	// Group rows by control_id.
	type entryIdx struct {
		index int
	}
	indexMap := make(map[string]entryIdx)
	var entries []controlEntry

	for _, row := range rows {
		sev := ""
		if row.Severity != nil {
			sev = *row.Severity
		}
		section := ""
		if row.Section != nil {
			section = *row.Section
		}
		fm := controlFrameworkMapping{
			BundleID:   row.BundleID,
			BundleName: row.BundleName,
			Section:    section,
		}
		if idx, ok := indexMap[row.ControlID]; ok {
			entries[idx.index].Frameworks = append(entries[idx.index].Frameworks, fm)
		} else {
			entries = append(entries, controlEntry{
				ControlID:      row.ControlID,
				Name:           row.Name,
				Severity:       sev,
				Engine:         row.Engine,
				EngineVersions: row.EngineVersions,
				Tags:           row.Tags,
				Frameworks:     []controlFrameworkMapping{fm},
			})
			indexMap[row.ControlID] = entryIdx{index: len(entries) - 1}
		}
	}

	// Apply pagination on the grouped result.
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(entries) {
		start = len(entries)
	}
	if end > len(entries) {
		end = len(entries)
	}
	paged := entries[start:end]
	if paged == nil {
		paged = []controlEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": paged,
		"total": total,
	})
}

// bundleManifest is the parsed bundle.yaml from inside a tarball.
type bundleManifest struct {
	ID        string   `yaml:"id"`
	Name      string   `yaml:"name"`
	Version   string   `yaml:"version"`
	Framework string   `yaml:"framework"`
	Engine    string   `yaml:"engine"`
	Controls  []string `yaml:"controls"`
}

// controlManifest is the parsed control.yaml from inside a tarball.
type controlManifest struct {
	ID       string `yaml:"id"`
	Title    string `yaml:"title"`
	Section  string `yaml:"section"`
	Severity string `yaml:"severity"`
}

// Upload handles POST /api/v1/bundles/upload (tenant-authed).
// Accepts multipart form with a "tarball" field containing the .tar.gz bundle.
// Parses bundle.yaml + control yamls, upserts DB rows, stores tarball locally.
func (h *BundlesHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "tarball too large or invalid multipart form")
		return
	}

	file, _, err := r.FormFile("tarball")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing tarball field")
		return
	}
	defer file.Close()

	// Read the entire tarball into memory (bounded by maxUploadSize).
	tarballData, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read tarball")
		return
	}

	// Parse bundle.yaml and control yamls from the tarball.
	manifest, controlFiles, err := parseBundleTarball(tarballData)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid bundle tarball: %s", err))
		return
	}

	// Compute SHA-256 hash of the tarball for verification.
	hashSum := sha256.Sum256(tarballData)
	tarballHash := hex.EncodeToString(hashSum[:])

	// Build the bundle model.
	engine := manifest.Engine
	b := model.Bundle{
		ID:           "", // let DB generate UUID; upsert by name+version
		Name:         manifest.Name,
		Version:      manifest.Version,
		Framework:    manifest.Framework,
		TargetType:   manifest.Engine, // engine doubles as target_type for compatibility
		Engine:       &engine,
		ControlCount: len(manifest.Controls),
		TarballHash:  &tarballHash,
	}

	// Upsert the bundle row.
	out, err := h.store.UpsertBundle(r.Context(), b)
	if err != nil {
		slog.Error("upserting bundle from upload", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save bundle")
		return
	}

	// Build control rows from parsed control yamls.
	controls := buildControlRows(out.ID, manifest, controlFiles)

	// Replace bundle_controls rows.
	if err := h.store.ReplaceBundleControls(r.Context(), out.ID, controls); err != nil {
		slog.Error("replacing bundle controls", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save controls")
		return
	}

	// Store tarball locally (v1 — no GCS).
	if h.storagePath != "" {
		if err := storeTarball(h.storagePath, manifest.Name, manifest.Version, tarballData); err != nil {
			slog.Warn("failed to store tarball locally", "error", err)
			// Non-fatal: DB registration is the primary job in v1.
		}
	}

	slog.Info("bundle uploaded",
		"bundle_id", out.ID, "name", out.Name, "version", out.Version,
		"controls", len(controls))

	resp := struct {
		Bundle   *model.Bundle          `json:"bundle"`
		Controls []model.BundleControl  `json:"controls"`
	}{
		Bundle:   out,
		Controls: controls,
	}
	writeJSON(w, http.StatusOK, resp)
}

// parseBundleTarball reads a .tar.gz and extracts bundle.yaml and all
// control.yaml files (from controls/<id>/control.yaml or
// content/controls/<section>.yaml for legacy bundles).
func parseBundleTarball(data []byte) (*bundleManifest, map[string]*controlManifest, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("decompressing gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var manifest *bundleManifest
	controlFiles := make(map[string]*controlManifest)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("reading tar: %w", err)
		}

		name := strings.TrimPrefix(hdr.Name, "./")

		// bundle.yaml at root
		if name == "bundle.yaml" {
			content, err := io.ReadAll(tr)
			if err != nil {
				return nil, nil, fmt.Errorf("reading bundle.yaml: %w", err)
			}
			m, err := parseBundleYAML(content)
			if err != nil {
				return nil, nil, fmt.Errorf("parsing bundle.yaml: %w", err)
			}
			manifest = m
			continue
		}

		// controls/<id>/control.yaml (ADR 010 format)
		if strings.HasPrefix(name, "controls/") && strings.HasSuffix(name, "/control.yaml") {
			parts := strings.Split(name, "/")
			if len(parts) == 3 {
				content, err := io.ReadAll(tr)
				if err != nil {
					slog.Warn("skipping unreadable control.yaml", "path", name, "error", err)
					continue
				}
				cm := parseControlYAML(content)
				controlID := parts[1]
				controlFiles[controlID] = cm
			}
			continue
		}

		// content/controls/<section>.yaml (legacy format)
		if strings.HasPrefix(name, "content/controls/") && strings.HasSuffix(name, ".yaml") {
			content, err := io.ReadAll(tr)
			if err != nil {
				slog.Warn("skipping unreadable legacy control yaml", "path", name, "error", err)
				continue
			}
			cm := parseControlYAML(content)
			base := strings.TrimSuffix(filepath.Base(name), ".yaml")
			controlFiles[base] = cm
			continue
		}
	}

	if manifest == nil {
		return nil, nil, fmt.Errorf("bundle.yaml not found in tarball")
	}
	if manifest.ID == "" || manifest.Name == "" || manifest.Version == "" {
		return nil, nil, fmt.Errorf("bundle.yaml missing required fields (id, name, version)")
	}

	return manifest, controlFiles, nil
}

// parseBundleYAML extracts fields from a bundle.yaml using line-based parsing.
// This avoids adding a yaml dependency to the api module.
func parseBundleYAML(data []byte) (*bundleManifest, error) {
	m := &bundleManifest{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inControls := false
	for scanner.Scan() {
		line := scanner.Text()

		// Controls list items
		if inControls {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				ctrl := strings.TrimPrefix(trimmed, "- ")
				ctrl = strings.Trim(ctrl, "\"' ")
				if ctrl != "" {
					m.Controls = append(m.Controls, ctrl)
				}
				continue
			}
			// Non-list line ends the controls block
			if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inControls = false
			} else if trimmed == "" {
				continue
			} else {
				inControls = false
			}
		}

		if strings.HasPrefix(line, "controls:") {
			inControls = true
			continue
		}

		if k, v, ok := parseYAMLLine(line); ok {
			switch k {
			case "id":
				m.ID = v
			case "name":
				m.Name = v
			case "version":
				m.Version = v
			case "framework":
				m.Framework = v
			case "engine":
				m.Engine = v
			}
		}
	}
	return m, scanner.Err()
}

// parseControlYAML extracts fields from a control.yaml using line-based parsing.
func parseControlYAML(data []byte) *controlManifest {
	cm := &controlManifest{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if k, v, ok := parseYAMLLine(line); ok {
			switch k {
			case "id":
				cm.ID = v
			case "title":
				cm.Title = v
			case "section":
				cm.Section = v
			case "severity":
				cm.Severity = v
			}
		}
	}
	return cm
}

// parseYAMLLine extracts a top-level "key: value" pair from a YAML line.
// Returns empty if the line is indented, a comment, or not a key-value pair.
func parseYAMLLine(line string) (key, value string, ok bool) {
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	// Strip quotes
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
		(value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
	}
	// Skip multiline markers
	if value == ">" || value == "|" || value == ">-" || value == "|-" {
		return "", "", false
	}
	return key, value, key != ""
}

// buildControlRows maps parsed control yamls to BundleControl model rows.
// For controls listed in the manifest but without a parsed control.yaml,
// a stub row is created with the control ID as the name.
func buildControlRows(bundleID string, manifest *bundleManifest, controlFiles map[string]*controlManifest) []model.BundleControl {
	var controls []model.BundleControl
	engine := manifest.Engine
	if engine == "" {
		engine = "unknown"
	}

	for _, ctrlID := range manifest.Controls {
		bc := model.BundleControl{
			BundleID:       bundleID,
			ControlID:      ctrlID,
			Name:           ctrlID, // default: overwritten if control.yaml found
			Engine:         engine,
			EngineVersions: json.RawMessage(`[]`),
			Tags:           json.RawMessage(`[]`),
		}

		// Try to find a matching control.yaml by control ID or by filename key.
		if cm, ok := controlFiles[ctrlID]; ok {
			populateControlFromManifest(&bc, cm)
		}

		controls = append(controls, bc)
	}
	return controls
}

func populateControlFromManifest(bc *model.BundleControl, cm *controlManifest) {
	if cm.Title != "" {
		bc.Name = cm.Title
	}
	if cm.Severity != "" {
		sev := strings.ToLower(cm.Severity)
		bc.Severity = &sev
	}
	if cm.Section != "" {
		bc.Section = &cm.Section
	}
}

// storeTarball writes the tarball to the local filesystem for v1.
func storeTarball(basePath, name, version string, data []byte) error {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return fmt.Errorf("creating storage directory: %w", err)
	}
	filename := fmt.Sprintf("%s-%s.tar.gz", name, version)
	path := filepath.Join(basePath, filename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing tarball: %w", err)
	}
	return nil
}

// UpsertBundle — internal (backoffice-authed). Used to seed global bundles
// (tenant_id NULL) that every tenant can use. Payload mirrors model.Bundle.
// Note: this lives on InternalHandler in main.go to reuse X-API-Key auth.
type UpsertBundleRequest struct {
	ID           string  `json:"id"`
	TenantID     *string `json:"tenant_id,omitempty"`
	Name         string  `json:"name"`
	Version      string  `json:"version"`
	Framework    string  `json:"framework"`
	TargetType   string  `json:"target_type"`
	Engine       *string `json:"engine,omitempty"`
	ControlCount int     `json:"control_count,omitempty"`
	GCSPath      *string `json:"gcs_path,omitempty"`
	Signature    *string `json:"signature,omitempty"`
	TarballHash  *string `json:"tarball_hash,omitempty"`
}

// InternalUpsertBundle is mounted on the /internal/v1 mux.
func (h *InternalHandler) UpsertBundle(w http.ResponseWriter, r *http.Request) {
	var req UpsertBundleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Version == "" || req.Framework == "" || req.TargetType == "" {
		writeError(w, http.StatusBadRequest, "name, version, framework, and target_type are required")
		return
	}
	b := model.Bundle{
		ID:           req.ID,
		TenantID:     req.TenantID,
		Name:         req.Name,
		Version:      req.Version,
		Framework:    req.Framework,
		TargetType:   req.TargetType,
		Engine:       req.Engine,
		ControlCount: req.ControlCount,
		GCSPath:      req.GCSPath,
		Signature:    req.Signature,
		TarballHash:  req.TarballHash,
	}
	out, err := h.store.UpsertBundle(r.Context(), b)
	if err != nil {
		slog.Error("upserting bundle", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to upsert bundle")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
