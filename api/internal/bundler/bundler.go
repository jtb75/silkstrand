// Package bundler assembles signed bundle tarballs from individual controls.
// It is the server-side equivalent of scripts/build-bundle.sh.
package bundler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuildOptions configures a bundle assembly.
type BuildOptions struct {
	BundleID    string   // UUID for the bundle row
	Name        string   // e.g., "ACME Database Hardening"
	Version     string   // e.g., "1.0.0"
	Framework   string   // e.g., "custom" or the base_framework
	Engine      string   // e.g., "postgresql" (derived from controls)
	ControlIDs  []string // list of control IDs to include
	ControlsDir string   // path to controls/ directory
	SigningKey  []byte   // optional; nil = unsigned (HMAC-SHA256)
}

// BuildResult is the output of a successful Build call.
type BuildResult struct {
	Tarball      []byte
	Hash         string // SHA256 hex of the tarball
	Signature    string // HMAC-SHA256 hex if signing key provided; empty otherwise
	ControlCount int
}

// Build assembles a signed bundle tarball from a list of control IDs.
// It reads control files from controlsDir, assembles them into a tar.gz
// in memory, optionally signs with the provided key, and returns the
// tarball bytes + computed SHA256 hash.
func Build(opts BuildOptions) (*BuildResult, error) {
	if len(opts.ControlIDs) == 0 {
		return nil, fmt.Errorf("no control IDs provided")
	}
	if opts.ControlsDir == "" {
		return nil, fmt.Errorf("controls directory not configured")
	}

	// Verify all controls exist before assembling.
	for _, cid := range opts.ControlIDs {
		dir := filepath.Join(opts.ControlsDir, cid)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("control directory not found: %s", cid)
		}
	}

	// Generate bundle.yaml manifest.
	manifest := generateManifest(opts)

	// Build the tarball in memory.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add bundle.yaml at the root.
	if err := addTarFile(tw, "bundle.yaml", manifest); err != nil {
		return nil, fmt.Errorf("adding bundle.yaml: %w", err)
	}

	// Add each control's files under controls/<id>/
	for _, cid := range opts.ControlIDs {
		ctrlDir := filepath.Join(opts.ControlsDir, cid)
		prefix := "controls/" + cid + "/"
		if err := addTarDir(tw, ctrlDir, prefix); err != nil {
			return nil, fmt.Errorf("adding control %s: %w", cid, err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("closing gzip writer: %w", err)
	}

	tarball := buf.Bytes()

	// Compute SHA-256 hash.
	hashSum := sha256.Sum256(tarball)
	hashHex := hex.EncodeToString(hashSum[:])

	result := &BuildResult{
		Tarball:      tarball,
		Hash:         hashHex,
		ControlCount: len(opts.ControlIDs),
	}

	// Sign if a key was provided (HMAC-SHA256, matching build-bundle.sh's
	// openssl dgst -sha256 scheme for server-side use).
	if len(opts.SigningKey) > 0 {
		sig := hmacSHA256(opts.SigningKey, tarball)
		result.Signature = hex.EncodeToString(sig)
	}

	return result, nil
}

// generateManifest produces a bundle.yaml string from build options.
func generateManifest(opts BuildOptions) []byte {
	var sb strings.Builder
	sb.WriteString("id: " + opts.BundleID + "\n")
	sb.WriteString("name: " + opts.Name + "\n")
	sb.WriteString("version: " + opts.Version + "\n")
	sb.WriteString("framework: " + opts.Framework + "\n")
	sb.WriteString("engine: " + opts.Engine + "\n")
	sb.WriteString("controls:\n")
	for _, cid := range opts.ControlIDs {
		sb.WriteString("  - " + cid + "\n")
	}
	return []byte(sb.String())
}

// addTarFile adds a single in-memory file to the tar archive.
func addTarFile(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// addTarDir walks a filesystem directory and adds all regular files to
// the tar under the given prefix.
func addTarDir(tw *tar.Writer, dir, prefix string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("computing relative path: %w", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		return addTarFile(tw, prefix+rel, data)
	})
}
