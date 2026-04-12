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

func TestPythonRunner_Run(t *testing.T) {
	// Skip if python3 is not available
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata", "test-bundle")

	manifest, err := LoadManifest(testdataDir)
	if err != nil {
		t.Fatalf("loading manifest: %v", err)
	}

	if manifest.Name != "test-bundle" {
		t.Errorf("manifest.Name = %q, want %q", manifest.Name, "test-bundle")
	}
	if manifest.Entrypoint != "content/checks.py" {
		t.Errorf("manifest.Entrypoint = %q, want %q", manifest.Entrypoint, "content/checks.py")
	}

	runner := NewPythonRunner()
	targetConfig := json.RawMessage(`{"type": "database", "identifier": "localhost:5432"}`)

	results, err := runner.Run(context.Background(), RunRequest{
		BundlePath:   testdataDir,
		Manifest:     manifest,
		TargetConfig: targetConfig,
	})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Validate results structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(results, &parsed); err != nil {
		t.Fatalf("results not valid JSON: %v", err)
	}

	if parsed["schema_version"] != "1" {
		t.Errorf("schema_version = %v, want %q", parsed["schema_version"], "1")
	}
	if parsed["status"] != "completed" {
		t.Errorf("status = %v, want %q", parsed["status"], "completed")
	}

	summary, ok := parsed["summary"].(map[string]interface{})
	if !ok {
		t.Fatal("missing summary in results")
	}
	if summary["total"] != float64(1) {
		t.Errorf("summary.total = %v, want 1", summary["total"])
	}
	if summary["pass"] != float64(1) {
		t.Errorf("summary.pass = %v, want 1", summary["pass"])
	}
}

func TestLoadManifest(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata", "test-bundle")

	m, err := LoadManifest(testdataDir)
	if err != nil {
		t.Fatalf("loading manifest: %v", err)
	}

	if m.Name != "test-bundle" {
		t.Errorf("Name = %q, want %q", m.Name, "test-bundle")
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", m.Version, "1.0.0")
	}
	if m.Framework != "python" {
		t.Errorf("Framework = %q, want %q", m.Framework, "python")
	}
	if m.TargetType != "database" {
		t.Errorf("TargetType = %q, want %q", m.TargetType, "database")
	}
	if m.Entrypoint != "content/checks.py" {
		t.Errorf("Entrypoint = %q, want %q", m.Entrypoint, "content/checks.py")
	}
	if m.Benchmark.CISID != "CIS_TEST" {
		t.Errorf("Benchmark.CISID = %q, want %q", m.Benchmark.CISID, "CIS_TEST")
	}
}

func TestLoadManifest_MissingEntrypoint(t *testing.T) {
	tmpDir := t.TempDir()
	data := []byte("name: test\nversion: 1.0.0\nframework: python\n")
	if err := writeTestFile(tmpDir, "manifest.yaml", data); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing entrypoint")
	}
}

func TestLoadManifest_MissingFile(t *testing.T) {
	_, err := LoadManifest(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing manifest file")
	}
}

func writeTestFile(dir, name string, data []byte) error {
	return os.WriteFile(filepath.Join(dir, name), data, 0o644)
}
