package cache

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
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
	publicKey ed25519.PublicKey // if nil, signature verification is skipped (dev mode)
}

// New creates a new Cache rooted at bundleDir.
// If publicKey is nil, signature verification is disabled (local dev only).
func New(bundleDir string, publicKey ed25519.PublicKey) *Cache {
	return &Cache{bundleDir: bundleDir, publicKey: publicKey}
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
		if err := c.verify(versionedPath); err != nil {
			return "", err
		}
		return versionedPath, nil
	}

	// Check flat layout (local dev convenience)
	flatPath := filepath.Join(c.bundleDir, bundleName)
	if _, err := os.Stat(filepath.Join(flatPath, "manifest.yaml")); err == nil {
		if err := c.verify(flatPath); err != nil {
			return "", err
		}
		return flatPath, nil
	}

	return "", ErrNotCached
}

// verify checks the Ed25519 signature of a bundle's manifest.
// If no public key is configured, verification is skipped (dev mode).
func (c *Cache) verify(bundlePath string) error {
	if c.publicKey == nil {
		return nil // dev mode — no verification
	}

	sigPath := filepath.Join(bundlePath, "signature.sig")
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("bundle missing signature file: %w", err)
	}

	manifestPath := filepath.Join(bundlePath, "manifest.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest for verification: %w", err)
	}

	if !ed25519.Verify(c.publicKey, manifest, sig) {
		return fmt.Errorf("bundle signature verification failed")
	}

	return nil
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
