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

// Runner executes a compliance bundle and returns structured results.
type Runner interface {
	Run(ctx context.Context, req RunRequest) (json.RawMessage, error)
}
