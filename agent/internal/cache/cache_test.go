package cache

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGet_VersionedLayout(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "my-bundle", "1.0.0")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.yaml"), []byte("name: my-bundle"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := New(tmpDir, nil)
	path, err := c.Get("my-bundle", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != bundleDir {
		t.Errorf("path = %q, want %q", path, bundleDir)
	}
}

func TestGet_FlatLayout(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "my-bundle")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.yaml"), []byte("name: my-bundle"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := New(tmpDir, nil)
	path, err := c.Get("my-bundle", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != bundleDir {
		t.Errorf("path = %q, want %q", path, bundleDir)
	}
}

func TestGet_NotCached(t *testing.T) {
	tmpDir := t.TempDir()
	c := New(tmpDir, nil)

	_, err := c.Get("nonexistent", "1.0.0")
	if !errors.Is(err, ErrNotCached) {
		t.Errorf("err = %v, want ErrNotCached", err)
	}
}

func TestGet_VersionedPreferredOverFlat(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both layouts
	flatDir := filepath.Join(tmpDir, "my-bundle")
	versionedDir := filepath.Join(tmpDir, "my-bundle", "2.0.0")

	if err := os.MkdirAll(versionedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flatDir, "manifest.yaml"), []byte("name: my-bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionedDir, "manifest.yaml"), []byte("name: my-bundle"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := New(tmpDir, nil)
	path, err := c.Get("my-bundle", "2.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != versionedDir {
		t.Errorf("path = %q, want versioned path %q", path, versionedDir)
	}
}
