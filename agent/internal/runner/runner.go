package runner

import (
	"context"
	"encoding/json"
)

// RunRequest contains everything needed to execute a compliance bundle.
type RunRequest struct {
	BundlePath   string
	Manifest     *Manifest
	TargetConfig json.RawMessage
	Credentials  json.RawMessage
}

// ControlRunRequest contains everything needed to execute a single control.
type ControlRunRequest struct {
	BundlePath   string
	ControlID    string
	Entrypoint   string // e.g. "controls/pg-tls-enabled/check.py"
	VendorDir    string // optional PYTHONPATH prefix
	TargetConfig json.RawMessage
	Credentials  json.RawMessage
}

// Runner executes a compliance bundle and returns structured results.
type Runner interface {
	// Run executes a legacy monolithic bundle entrypoint.
	Run(ctx context.Context, req RunRequest) (json.RawMessage, error)

	// RunControl executes a single control's check.py entrypoint and
	// returns its JSON result. Used by the manifest-aware execution loop.
	RunControl(ctx context.Context, req ControlRunRequest) (json.RawMessage, error)
}
