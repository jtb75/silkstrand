package rules

import (
	"encoding/json"
	"errors"
	"testing"
)

// P1 smoke test: the predicate evaluator is deliberately a stub between
// P1 (schema) and P2 (ingest rewiring). Match must return the sentinel
// error so callers know to skip rule evaluation rather than treating
// "no match" as a pass-through.
func TestMatchReturnsNotImplemented(t *testing.T) {
	ok, err := Match(json.RawMessage(`{"service":"postgresql"}`), nil)
	if ok {
		t.Fatal("stub Match returned true")
	}
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
}
