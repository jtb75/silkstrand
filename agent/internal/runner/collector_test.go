package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetermineCollector(t *testing.T) {
	tests := []struct {
		targetType string
		want       string
	}{
		{"mssql", "mssql-collector"},
		{"MSSQL", "mssql-collector"},
		{"postgresql", "postgresql-collector"},
		{"mongodb", "mongodb-collector"},
		{"database", ""},
		{"unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.targetType, func(t *testing.T) {
			got := DetermineCollector(tt.targetType)
			if got != tt.want {
				t.Errorf("DetermineCollector(%q) = %q, want %q", tt.targetType, got, tt.want)
			}
		})
	}
}

func TestEnsureCollector_Fallback_PATH(t *testing.T) {
	// "echo" should always be on PATH.
	// Override the runtimes dir to a non-existent location so GCS/cache
	// paths are skipped.
	origDir := collectorRuntimesDir
	collectorRuntimesDir = t.TempDir()
	defer func() { collectorRuntimesDir = origDir }()

	// DetermineCollector won't help here — we test EnsureCollector
	// directly with a tool name that exists on PATH.
	path, err := exec.LookPath("echo")
	if err != nil {
		t.Skip("echo not on PATH")
	}

	// Temporarily add "echo" to the collector map.
	collectorMap["_test_echo"] = "echo"
	defer delete(collectorMap, "_test_echo")

	// Suppress the GCS download attempt by pointing at a bad URL.
	origBase := collectorRuntimesBaseURL
	collectorRuntimesBaseURL = "http://localhost:1/nonexistent"
	defer func() { collectorRuntimesBaseURL = origBase }()

	got, err := EnsureCollector("echo")
	if err != nil {
		t.Fatalf("EnsureCollector(echo) error: %v", err)
	}
	if got != path {
		t.Errorf("got %q, want %q", got, path)
	}
}

func TestEnsureCollector_Fallback_BundleDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake collector binary in the bundles dir.
	fakeBin := filepath.Join(tmpDir, "test-collector")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origDir := collectorRuntimesDir
	collectorRuntimesDir = t.TempDir()
	defer func() { collectorRuntimesDir = origDir }()

	origBase := collectorRuntimesBaseURL
	collectorRuntimesBaseURL = "http://localhost:1/nonexistent"
	defer func() { collectorRuntimesBaseURL = origBase }()

	origBundle := os.Getenv("SILKSTRAND_BUNDLE_DIR")
	os.Setenv("SILKSTRAND_BUNDLE_DIR", tmpDir)
	defer os.Setenv("SILKSTRAND_BUNDLE_DIR", origBundle)

	got, err := EnsureCollector("test-collector")
	if err != nil {
		t.Fatalf("EnsureCollector error: %v", err)
	}
	if got != fakeBin {
		t.Errorf("got %q, want %q", got, fakeBin)
	}
}

func TestRunCollector(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses shell script")
	}

	// Create a fake collector script that reads stdin and writes valid JSON.
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "fake-collector")
	scriptContent := `#!/bin/sh
cat <<'ENDJSON'
{"collector_id":"test-collector","facts":{"key1":"value1","key2":42}}
ENDJSON
`
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	targetConfig := json.RawMessage(`{"host":"localhost","port":1433}`)
	creds := json.RawMessage(`{"username":"sa","password":"test"}`)

	result, err := RunCollector(context.Background(), script, targetConfig, creds)
	if err != nil {
		t.Fatalf("RunCollector error: %v", err)
	}

	if result.CollectorID != "test-collector" {
		t.Errorf("collector_id = %q, want %q", result.CollectorID, "test-collector")
	}
	if len(result.Facts) != 2 {
		t.Errorf("facts count = %d, want 2", len(result.Facts))
	}
	if result.Facts["key1"] != "value1" {
		t.Errorf("facts[key1] = %v, want value1", result.Facts["key1"])
	}
}

func TestRunCollector_Error(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses shell script")
	}

	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "fail-collector")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'connection refused' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := RunCollector(context.Background(), script, json.RawMessage(`{}`), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from failing collector")
	}
	if got := err.Error(); !contains(got, "connection refused") {
		t.Errorf("error should contain stderr output, got: %s", got)
	}
}

func TestRunCollector_NoFacts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses shell script")
	}

	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "empty-collector")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{\"collector_id\":\"x\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := RunCollector(context.Background(), script, json.RawMessage(`{}`), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for nil facts")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
