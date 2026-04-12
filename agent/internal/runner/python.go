package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const pythonTimeout = 5 * time.Minute

// PythonRunner executes Python-based compliance bundles.
type PythonRunner struct{}

// NewPythonRunner creates a new PythonRunner.
func NewPythonRunner() *PythonRunner {
	return &PythonRunner{}
}

// Run executes a Python compliance bundle and returns the results as JSON.
func (r *PythonRunner) Run(ctx context.Context, req RunRequest) (json.RawMessage, error) {
	tmpDir, err := os.MkdirTemp("", "silkstrand-scan-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write target config
	targetConfigPath := filepath.Join(tmpDir, "target_config.json")
	if err := os.WriteFile(targetConfigPath, req.TargetConfig, 0o600); err != nil {
		return nil, fmt.Errorf("writing target config: %w", err)
	}

	// Build command with timeout
	runCtx, cancel := context.WithTimeout(ctx, pythonTimeout)
	defer cancel()

	bundlePath, err := filepath.Abs(req.BundlePath)
	if err != nil {
		return nil, fmt.Errorf("resolving bundle path: %w", err)
	}
	entrypoint := filepath.Join(bundlePath, req.Manifest.Entrypoint)
	cmd := exec.CommandContext(runCtx, "python3", entrypoint)
	cmd.Dir = bundlePath

	env := os.Environ()
	env = append(env, "SILKSTRAND_TARGET_CONFIG="+targetConfigPath)

	// Write credentials if provided
	if len(req.Credentials) > 0 && string(req.Credentials) != "null" {
		credentialsPath := filepath.Join(tmpDir, "credentials.json")
		if err := os.WriteFile(credentialsPath, req.Credentials, 0o600); err != nil {
			return nil, fmt.Errorf("writing credentials: %w", err)
		}
		env = append(env, "SILKSTRAND_CREDENTIALS="+credentialsPath)
	}

	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Info("executing bundle", "entrypoint", req.Manifest.Entrypoint, "bundle", req.Manifest.Name)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("executing bundle: %w\nstderr: %s", err, stderr.String())
	}

	if stderr.Len() > 0 {
		slog.Debug("bundle stderr output", "stderr", stderr.String())
	}

	// Validate stdout is valid JSON
	output := stdout.Bytes()
	if !json.Valid(output) {
		return nil, fmt.Errorf("bundle output is not valid JSON: %s", string(output))
	}

	return json.RawMessage(output), nil
}
