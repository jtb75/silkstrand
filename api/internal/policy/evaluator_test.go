package policy

import (
	"context"
	"testing"
)

const testRegoPass = `package silkstrand.test.always_pass

import rego.v1

default result := {"control_id": "test-pass", "status": "fail", "severity": "high", "title": "Test"}

result := r if {
	input.facts.enabled
	r := {"control_id": "test-pass", "status": "pass", "severity": "high", "title": "Test"}
}
`

const testRegoFail = `package silkstrand.test.always_fail

import rego.v1

result := {"control_id": "test-fail", "status": "fail", "severity": "medium", "title": "Always fails", "remediation": "Fix it"}
`

func TestLoadAndEvaluatePass(t *testing.T) {
	e := NewEvaluator()
	if err := e.Load("test-pass", testRegoPass, "data.silkstrand.test.always_pass.result"); err != nil {
		t.Fatal(err)
	}
	results := e.Evaluate(context.Background(), map[string]any{"enabled": true})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "pass" {
		t.Errorf("expected pass, got %s", results[0].Status)
	}
}

func TestLoadAndEvaluateFail(t *testing.T) {
	e := NewEvaluator()
	if err := e.Load("test-pass", testRegoPass, "data.silkstrand.test.always_pass.result"); err != nil {
		t.Fatal(err)
	}
	results := e.Evaluate(context.Background(), map[string]any{"enabled": false})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "fail" {
		t.Errorf("expected fail, got %s", results[0].Status)
	}
}

func TestInvalidRego(t *testing.T) {
	e := NewEvaluator()
	err := e.Load("bad", "this is not valid rego", "data.bad.result")
	if err == nil {
		t.Fatal("expected error for invalid rego")
	}
}

func TestMultiplePolicies(t *testing.T) {
	e := NewEvaluator()
	if err := e.Load("test-pass", testRegoPass, "data.silkstrand.test.always_pass.result"); err != nil {
		t.Fatal(err)
	}
	if err := e.Load("test-fail", testRegoFail, "data.silkstrand.test.always_fail.result"); err != nil {
		t.Fatal(err)
	}
	if e.Count() != 2 {
		t.Fatalf("expected 2 policies, got %d", e.Count())
	}
	results := e.Evaluate(context.Background(), map[string]any{"enabled": true})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestReload(t *testing.T) {
	e := NewEvaluator()
	if err := e.Load("test-pass", testRegoPass, "data.silkstrand.test.always_pass.result"); err != nil {
		t.Fatal(err)
	}
	// Reload with the always-fail policy under the same ID
	if err := e.Load("test-pass", testRegoFail, "data.silkstrand.test.always_fail.result"); err != nil {
		t.Fatal(err)
	}
	if e.Count() != 1 {
		t.Fatalf("expected 1 policy after reload, got %d", e.Count())
	}
	results := e.Evaluate(context.Background(), map[string]any{"enabled": true})
	if results[0].Status != "fail" {
		t.Errorf("expected fail after reload, got %s", results[0].Status)
	}
}

func TestEmptyEvaluator(t *testing.T) {
	e := NewEvaluator()
	results := e.Evaluate(context.Background(), map[string]any{"foo": "bar"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results from empty evaluator, got %d", len(results))
	}
}

func TestDeriveQuery(t *testing.T) {
	q := deriveQuery(testRegoPass)
	if q != "data.silkstrand.test.always_pass.result" {
		t.Errorf("got %q", q)
	}
}
