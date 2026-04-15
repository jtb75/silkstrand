// Package rules implements the ADR 003 D2 generalized match → action
// engine. Predicates are JSONB documents stored in correlation_rules.body
// (behind a `collection_id` indirection post-ADR-006 D6) and in
// collections.predicate.
//
// P1 NOTE: under the asset-first refactor the predicate evaluator must
// dispatch on collection scope (asset / endpoint / finding) to resolve
// the right field set, per ADR 006 D5. The legacy evaluator was bolted to
// *model.DiscoveredAsset; that type is deleted in migration 017. Wiring
// the evaluator to the new Asset / AssetEndpoint / Finding shape is P2
// work (see docs/plans/asset-first-execution.md). For now Match is a
// stub that always returns (false, errNotImplemented) — callers must
// gate on this and skip rule evaluation until P2 lands.
package rules

import (
	"encoding/json"
	"errors"
)

// ErrNotImplemented is returned by Match while the asset-first refactor
// is between P1 (schema) and P2 (ingest + rules rewiring).
var ErrNotImplemented = errors.New("predicate matcher not implemented in P1; landing in P2")

// Match evaluates a predicate JSONB against a subject. The subject is
// left as `any` so that a single entry point can dispatch to per-scope
// field extractors (asset / endpoint / finding) once P2 wires the
// evaluator.
//
// In P1 Match always returns (false, ErrNotImplemented). The rule
// engine logs and skips rather than treating this as a match — see
// EvaluateAsset in engine.go.
func Match(predicate json.RawMessage, subject any) (bool, error) {
	_ = predicate
	_ = subject
	return false, ErrNotImplemented
}
