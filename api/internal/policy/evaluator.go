// Package policy provides an in-process OPA Rego evaluator for
// compliance policy rules. Policies are pre-compiled at load time
// and evaluated against collected facts from engine collectors.
//
// See ADR 011: collector + policy split.
package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/rego"
)

// Result is one policy evaluation outcome — pass, fail, or error.
type Result struct {
	ControlID   string         `json:"control_id"`
	Status      string         `json:"status"` // pass | fail | error
	Severity    string         `json:"severity,omitempty"`
	Title       string         `json:"title,omitempty"`
	Evidence    map[string]any `json:"evidence,omitempty"`
	Remediation string         `json:"remediation,omitempty"`
}

// Policy is a single compiled Rego rule.
type Policy struct {
	ControlID  string
	RegoSource string
	Query      string
	prepared   rego.PreparedEvalQuery
}

// Evaluator holds pre-compiled Rego policies and evaluates facts
// against them. Thread-safe for concurrent evaluations.
type Evaluator struct {
	mu       sync.RWMutex
	policies map[string]*Policy
}

// NewEvaluator creates an empty evaluator. Load policies via Load or
// LoadFromDir before calling Evaluate.
func NewEvaluator() *Evaluator {
	return &Evaluator{policies: make(map[string]*Policy)}
}

// Load compiles a Rego policy and adds (or replaces) it in the evaluator.
func (e *Evaluator) Load(controlID, regoSource, query string) error {
	r := rego.New(
		rego.Query(query),
		rego.Module(controlID+".rego", regoSource),
	)
	pq, err := r.PrepareForEval(context.Background())
	if err != nil {
		return fmt.Errorf("compiling policy %s: %w", controlID, err)
	}
	e.mu.Lock()
	e.policies[controlID] = &Policy{
		ControlID:  controlID,
		RegoSource: regoSource,
		Query:      query,
		prepared:   pq,
	}
	e.mu.Unlock()
	return nil
}

// Remove deletes a policy from the evaluator.
func (e *Evaluator) Remove(controlID string) {
	e.mu.Lock()
	delete(e.policies, controlID)
	e.mu.Unlock()
}

// Count returns the number of loaded policies.
func (e *Evaluator) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.policies)
}

// Evaluate runs all loaded policies against the given facts and returns
// one Result per policy. Never returns an error for the batch — individual
// policy failures produce Result{Status: "error"}.
func (e *Evaluator) Evaluate(ctx context.Context, facts map[string]any) []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()

	input := map[string]any{"facts": facts}
	results := make([]Result, 0, len(e.policies))

	for _, p := range e.policies {
		rs, err := p.prepared.Eval(ctx, rego.EvalInput(input))
		if err != nil {
			results = append(results, Result{
				ControlID: p.ControlID,
				Status:    "error",
				Title:     fmt.Sprintf("evaluation error: %v", err),
			})
			continue
		}
		r := parseResult(p.ControlID, rs)
		results = append(results, r)
	}
	return results
}

// LoadFromDir reads all policies/<id>/policy.rego files from the given
// directory and loads each into the evaluator. The OPA query is derived
// from the Rego package declaration: data.<package>.result.
func (e *Evaluator) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		slog.Info("policy directory not found, starting with no policies", "dir", dir)
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading policy dir: %w", err)
	}
	loaded := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		controlID := entry.Name()
		regoPath := filepath.Join(dir, controlID, "policy.rego")
		data, err := os.ReadFile(regoPath)
		if err != nil {
			slog.Warn("skipping policy dir (no policy.rego)", "control_id", controlID)
			continue
		}
		regoSource := string(data)
		query := deriveQuery(regoSource)
		if query == "" {
			slog.Warn("skipping policy (no package declaration)", "control_id", controlID)
			continue
		}
		if err := e.Load(controlID, regoSource, query); err != nil {
			slog.Error("failed to load policy", "control_id", controlID, "error", err)
			continue
		}
		loaded++
	}
	slog.Info("policies loaded", "count", loaded, "dir", dir)
	return nil
}

// deriveQuery extracts the package name from Rego source and builds
// the query "data.<package>.result".
func deriveQuery(regoSource string) string {
	for _, line := range strings.Split(regoSource, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			pkg := strings.TrimPrefix(line, "package ")
			pkg = strings.TrimSpace(pkg)
			return "data." + pkg + ".result"
		}
	}
	return ""
}

// parseResult converts OPA's evaluation result set into our Result struct.
func parseResult(controlID string, rs rego.ResultSet) Result {
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return Result{ControlID: controlID, Status: "error", Title: "no result from policy"}
	}
	val := rs[0].Expressions[0].Value

	// OPA returns the value as a map[string]interface{} matching our Result shape.
	raw, err := json.Marshal(val)
	if err != nil {
		return Result{ControlID: controlID, Status: "error", Title: "failed to marshal result"}
	}
	var r Result
	if err := json.Unmarshal(raw, &r); err != nil {
		return Result{ControlID: controlID, Status: "error", Title: "failed to parse result"}
	}
	if r.ControlID == "" {
		r.ControlID = controlID
	}
	return r
}
