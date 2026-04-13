package cache

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// GetOrFetch returns a cached bundle directory, downloading the tarball from
// url if the bundle isn't already present. Expects a sibling .sha256 file next
// to the tarball (same layout as our agent-binary releases) and verifies the
// download against it before extracting.
func (c *Cache) GetOrFetch(bundleName, version, url string) (string, error) {
	if path, err := c.Get(bundleName, version); err == nil {
		return path, nil
	} else if !errors.Is(err, ErrNotCached) {
		return "", err
	}
	if url == "" {
		return "", fmt.Errorf("bundle %s v%s not cached and no URL supplied", bundleName, version)
	}

	slog.Info("fetching bundle", "name", bundleName, "version", version, "url", url)

	client := &http.Client{Timeout: 2 * time.Minute}

	// Read the advertised checksum first; refuse to download without one.
	shaResp, err := client.Get(url + ".sha256")
	if err != nil {
		return "", fmt.Errorf("fetching bundle checksum: %w", err)
	}
	defer shaResp.Body.Close()
	if shaResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bundle checksum not found (HTTP %d) at %s.sha256", shaResp.StatusCode, url)
	}
	shaData, _ := io.ReadAll(io.LimitReader(shaResp.Body, 1<<12))
	expected := strings.TrimSpace(strings.Fields(string(shaData))[0])
	if len(expected) != 64 {
		return "", fmt.Errorf("invalid SHA-256 format at %s.sha256: %q", url, expected)
	}

	// Download the tarball.
	tarResp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching bundle: %w", err)
	}
	defer tarResp.Body.Close()
	if tarResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bundle download returned HTTP %d for %s", tarResp.StatusCode, url)
	}

	h := sha256.New()
	body, err := io.ReadAll(io.TeeReader(tarResp.Body, h))
	if err != nil {
		return "", fmt.Errorf("reading bundle body: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return "", fmt.Errorf("bundle checksum mismatch: expected %s, got %s", expected, got)
	}

	path, err := c.Store(bundleName, version, body)
	if err != nil {
		return "", fmt.Errorf("storing bundle: %w", err)
	}
	// Run the existing signature check after extraction.
	if err := c.verify(path); err != nil {
		return "", err
	}
	slog.Info("cached bundle", "path", path)
	return path, nil
}
