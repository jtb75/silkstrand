package bundler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuild_BasicAssembly(t *testing.T) {
	// Set up a temp controls directory with one test control.
	tmpDir := t.TempDir()
	ctrlID := "pg-test-control"
	ctrlDir := filepath.Join(tmpDir, ctrlID)
	if err := os.MkdirAll(ctrlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctrlDir, "control.yaml"), []byte("id: pg-test-control\ntitle: Test Control\nseverity: high\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctrlDir, "check.py"), []byte("print('hello')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Build(BuildOptions{
		BundleID:    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Name:        "Test Bundle",
		Version:     "1.0.0",
		Framework:   "custom",
		Engine:      "postgresql",
		ControlIDs:  []string{ctrlID},
		ControlsDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify control count.
	if result.ControlCount != 1 {
		t.Errorf("expected 1 control, got %d", result.ControlCount)
	}

	// Verify hash matches SHA256 of the tarball.
	hashSum := sha256.Sum256(result.Tarball)
	expectedHash := hex.EncodeToString(hashSum[:])
	if result.Hash != expectedHash {
		t.Errorf("hash mismatch: got %s, want %s", result.Hash, expectedHash)
	}

	// Verify signature is empty when no signing key provided.
	if result.Signature != "" {
		t.Errorf("expected empty signature, got %s", result.Signature)
	}

	// Decompress and verify tarball contents.
	files := extractTarFiles(t, result.Tarball)

	// Must contain bundle.yaml.
	if _, ok := files["bundle.yaml"]; !ok {
		t.Error("tarball missing bundle.yaml")
	}

	// Must contain controls/<id>/check.py.
	checkKey := "controls/" + ctrlID + "/check.py"
	if _, ok := files[checkKey]; !ok {
		t.Errorf("tarball missing %s", checkKey)
	}

	// Must contain controls/<id>/control.yaml.
	ctrlKey := "controls/" + ctrlID + "/control.yaml"
	if _, ok := files[ctrlKey]; !ok {
		t.Errorf("tarball missing %s", ctrlKey)
	}

	// Verify bundle.yaml content.
	manifest := string(files["bundle.yaml"])
	if !strings.Contains(manifest, "name: Test Bundle") {
		t.Errorf("manifest missing name, got: %s", manifest)
	}
	if !strings.Contains(manifest, "engine: postgresql") {
		t.Errorf("manifest missing engine, got: %s", manifest)
	}
	if !strings.Contains(manifest, "  - pg-test-control") {
		t.Errorf("manifest missing control reference, got: %s", manifest)
	}
}

func TestBuild_WithSigningKey(t *testing.T) {
	tmpDir := t.TempDir()
	ctrlID := "test-ctrl"
	ctrlDir := filepath.Join(tmpDir, ctrlID)
	os.MkdirAll(ctrlDir, 0o755)
	os.WriteFile(filepath.Join(ctrlDir, "check.py"), []byte("pass"), 0o644)
	os.WriteFile(filepath.Join(ctrlDir, "control.yaml"), []byte("id: test-ctrl\n"), 0o644)

	key := []byte("test-signing-key-32-bytes-long!!")

	result, err := Build(BuildOptions{
		BundleID:    "11111111-2222-3333-4444-555555555555",
		Name:        "Signed Bundle",
		Version:     "1.0.0",
		Framework:   "custom",
		Engine:      "postgresql",
		ControlIDs:  []string{ctrlID},
		ControlsDir: tmpDir,
		SigningKey:   key,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if result.Signature == "" {
		t.Error("expected non-empty signature with signing key")
	}
	if len(result.Signature) != 64 { // SHA256 hex = 64 chars
		t.Errorf("unexpected signature length: %d", len(result.Signature))
	}
}

func TestBuild_MissingControl(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := Build(BuildOptions{
		BundleID:    "11111111-2222-3333-4444-555555555555",
		Name:        "Missing",
		Version:     "1.0.0",
		Framework:   "custom",
		Engine:      "postgresql",
		ControlIDs:  []string{"nonexistent-control"},
		ControlsDir: tmpDir,
	})
	if err == nil {
		t.Fatal("expected error for missing control")
	}
	if !strings.Contains(err.Error(), "control directory not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuild_NoControls(t *testing.T) {
	_, err := Build(BuildOptions{
		BundleID:    "11111111-2222-3333-4444-555555555555",
		Name:        "Empty",
		Version:     "1.0.0",
		Framework:   "custom",
		Engine:      "postgresql",
		ControlIDs:  nil,
		ControlsDir: "/tmp",
	})
	if err == nil {
		t.Fatal("expected error for no controls")
	}
}

func TestBuild_MultipleControls(t *testing.T) {
	tmpDir := t.TempDir()
	for _, cid := range []string{"ctrl-a", "ctrl-b", "ctrl-c"} {
		dir := filepath.Join(tmpDir, cid)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "check.py"), []byte("pass"), 0o644)
		os.WriteFile(filepath.Join(dir, "control.yaml"), []byte("id: "+cid+"\n"), 0o644)
	}

	result, err := Build(BuildOptions{
		BundleID:    "11111111-2222-3333-4444-555555555555",
		Name:        "Multi Control",
		Version:     "2.0.0",
		Framework:   "custom",
		Engine:      "mssql",
		ControlIDs:  []string{"ctrl-a", "ctrl-b", "ctrl-c"},
		ControlsDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if result.ControlCount != 3 {
		t.Errorf("expected 3 controls, got %d", result.ControlCount)
	}

	files := extractTarFiles(t, result.Tarball)
	for _, cid := range []string{"ctrl-a", "ctrl-b", "ctrl-c"} {
		if _, ok := files["controls/"+cid+"/check.py"]; !ok {
			t.Errorf("missing controls/%s/check.py", cid)
		}
	}
}

// extractTarFiles decompresses a tar.gz and returns a map of path → content.
func extractTarFiles(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	files := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("reading %s: %v", hdr.Name, err)
		}
		files[hdr.Name] = content
	}
	return files
}
