package cache

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotCached indicates the requested bundle is not in the local cache.
var ErrNotCached = errors.New("bundle not cached")

// Cache manages local storage of compliance bundles.
type Cache struct {
	bundleDir string
}

// New creates a new Cache rooted at bundleDir.
func New(bundleDir string) *Cache {
	return &Cache{bundleDir: bundleDir}
}

// Get returns the path to a cached bundle directory. It checks two layouts:
//  1. Versioned: {bundleDir}/{bundleName}/{version}/manifest.yaml
//  2. Flat (local dev): {bundleDir}/{bundleName}/manifest.yaml
//
// Returns ErrNotCached if neither layout has the bundle.
func (c *Cache) Get(bundleName, version string) (string, error) {
	// Check versioned layout first
	versionedPath := filepath.Join(c.bundleDir, bundleName, version)
	if _, err := os.Stat(filepath.Join(versionedPath, "manifest.yaml")); err == nil {
		return versionedPath, nil
	}

	// Check flat layout (local dev convenience)
	flatPath := filepath.Join(c.bundleDir, bundleName)
	if _, err := os.Stat(filepath.Join(flatPath, "manifest.yaml")); err == nil {
		return flatPath, nil
	}

	return "", ErrNotCached
}

// Store extracts a .tar.gz bundle archive into the cache at {bundleDir}/{bundleName}/{version}/.
func (c *Cache) Store(bundleName, version string, data []byte) (string, error) {
	destDir := filepath.Join(c.bundleDir, bundleName, version)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}

	r, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("opening gzip reader: %w", err)
	}
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(destDir, hdr.Name)

		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			return "", fmt.Errorf("invalid tar entry path: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", fmt.Errorf("creating parent directory: %w", err)
			}
			f, err := os.Create(target)
			if err != nil {
				return "", fmt.Errorf("creating file %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return "", fmt.Errorf("writing file %s: %w", target, err)
			}
			f.Close()
		}
	}

	return destDir, nil
}
