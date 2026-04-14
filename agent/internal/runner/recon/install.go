package recon

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// runtimesBaseURL is the public GCS bucket hosting PD binaries +
// curated nuclei-templates tarballs. Created in PR #1 (infra), seeded
// in PR #6 (binaries).
var runtimesBaseURL = func() string {
	if v := os.Getenv("SILKSTRAND_RUNTIMES_BASE_URL"); v != "" {
		return v
	}
	return "https://storage.googleapis.com/silkstrand-runtimes"
}()

// runtimesDir is where installed binaries live on disk. Override via
// SILKSTRAND_RUNTIMES_DIR for airgapped or test environments.
var runtimesDir = func() string {
	if v := os.Getenv("SILKSTRAND_RUNTIMES_DIR"); v != "" {
		return v
	}
	return "/var/lib/silkstrand/runtimes"
}()

// ErrUnsupportedPlatform indicates no PD binary is published for the
// agent's GOOS/GOARCH combination (e.g. windows/arm64). The recon
// directive is rejected with `platform_unsupported`; compliance scans
// are unaffected.
var ErrUnsupportedPlatform = errors.New("platform_unsupported")

// ErrPinsMissing means pdpins.go hasn't been populated yet (pre-PR #6).
// The recon directive is rejected; this is the expected state on first
// deploy.
var ErrPinsMissing = errors.New("pd_pins_missing")

// installMu serializes installs across goroutines for the same tool.
var installMu sync.Mutex

// EnsureTool returns the absolute path to a verified PD binary,
// fetching it from the runtimes bucket on first call. Subsequent calls
// re-verify the on-disk file's sha256 and reuse it.
func EnsureTool(name string) (string, error) {
	installMu.Lock()
	defer installMu.Unlock()

	tool, ok := lookupPin(name)
	if !ok {
		return "", fmt.Errorf("unknown PD tool: %s", name)
	}
	if tool.Version == "" {
		return "", ErrPinsMissing
	}
	platformKey := runtime.GOOS + "-" + runtime.GOARCH
	expectedSHA, ok := tool.SHA256[platformKey]
	if !ok || expectedSHA == "" {
		return "", fmt.Errorf("%w: %s on %s", ErrUnsupportedPlatform, name, platformKey)
	}

	exeName := name
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	dir := filepath.Join(runtimesDir, name, tool.Version)
	target := filepath.Join(dir, exeName)

	// Reuse if present and sha matches.
	if got, err := fileSHA256(target); err == nil && got == expectedSHA {
		return target, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating runtime dir: %w", err)
	}

	url := fmt.Sprintf("%s/%s/%s/%s/%s",
		runtimesBaseURL, name, tool.Version, platformKey, exeName)
	tmp := target + ".tmp"
	if err := downloadAndVerify(url, tmp, expectedSHA); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", fmt.Errorf("chmod: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", fmt.Errorf("atomic rename: %w", err)
	}
	return target, nil
}

func lookupPin(name string) (PDTool, bool) {
	for _, t := range pdTools {
		if t.Name == name {
			return t, true
		}
	}
	return PDTool{}, false
}

// EnsureTemplates returns the absolute path to the nuclei-templates
// directory, downloading + verifying the tarball on first call.
func EnsureTemplates() (string, error) {
	installMu.Lock()
	defer installMu.Unlock()

	if nucleiTemplatesPin.Version == "" {
		return "", ErrPinsMissing
	}
	dir := filepath.Join(runtimesDir, "nuclei-templates", nucleiTemplatesPin.Version)
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}
	// PR #6 will implement: download .tar.gz + .sig from
	// runtimesBaseURL/nuclei-templates/<version>.tar.gz, verify Ed25519
	// sig (reusing the bundle-signing pubkey), extract into dir.
	return "", fmt.Errorf("template fetch not implemented (PR #6)")
}

func downloadAndVerify(url, dst, expectedSHA string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hasher), resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", dst, err)
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if got != expectedSHA {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, expectedSHA)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
