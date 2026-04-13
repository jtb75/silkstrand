package config

import (
	"log/slog"
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Set required vars
	t.Setenv("SILKSTRAND_AGENT_ID", "test-agent")
	t.Setenv("SILKSTRAND_AGENT_KEY", "test-key")

	// Clear optional vars to test defaults
	t.Setenv("SILKSTRAND_API_URL", "")
	t.Setenv("SILKSTRAND_BUNDLE_DIR", "")
	t.Setenv("SILKSTRAND_LOG_LEVEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIURL != "ws://localhost:8080" {
		t.Errorf("APIURL = %q, want %q", cfg.APIURL, "ws://localhost:8080")
	}
	if cfg.BundleDir != "./bundles" {
		t.Errorf("BundleDir = %q, want %q", cfg.BundleDir, "./bundles")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("SILKSTRAND_API_URL", "wss://api.example.com")
	t.Setenv("SILKSTRAND_AGENT_ID", "my-agent")
	t.Setenv("SILKSTRAND_AGENT_KEY", "my-key")
	t.Setenv("SILKSTRAND_BUNDLE_DIR", "/opt/bundles")
	t.Setenv("SILKSTRAND_LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIURL != "wss://api.example.com" {
		t.Errorf("APIURL = %q, want %q", cfg.APIURL, "wss://api.example.com")
	}
	if cfg.AgentID != "my-agent" {
		t.Errorf("AgentID = %q, want %q", cfg.AgentID, "my-agent")
	}
	if cfg.AgentKey != "my-key" {
		t.Errorf("AgentKey = %q, want %q", cfg.AgentKey, "my-key")
	}
	if cfg.BundleDir != "/opt/bundles" {
		t.Errorf("BundleDir = %q, want %q", cfg.BundleDir, "/opt/bundles")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoad_MissingAgentID(t *testing.T) {
	// Unset to avoid inheriting from parent
	os.Unsetenv("SILKSTRAND_AGENT_ID")
	t.Setenv("SILKSTRAND_AGENT_KEY", "test-key")
	os.Unsetenv("SILKSTRAND_INSTALL_TOKEN")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for half-filled credential pair")
	}
}

func TestLoad_MissingAgentKey(t *testing.T) {
	t.Setenv("SILKSTRAND_AGENT_ID", "test-agent")
	os.Unsetenv("SILKSTRAND_AGENT_KEY")
	os.Unsetenv("SILKSTRAND_INSTALL_TOKEN")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for half-filled credential pair")
	}
}

// Install-token-only path (container / k8s bootstrap flow): Load must
// succeed; the caller is responsible for running bootstrap to populate
// the missing credential fields before use.
func TestLoad_InstallTokenOnly(t *testing.T) {
	os.Unsetenv("SILKSTRAND_AGENT_ID")
	os.Unsetenv("SILKSTRAND_AGENT_KEY")
	t.Setenv("SILKSTRAND_INSTALL_TOKEN", "sst_abc")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InstallToken != "sst_abc" {
		t.Errorf("InstallToken = %q, want %q", cfg.InstallToken, "sst_abc")
	}
}

// Load with nothing at all should still fail — no credentials, no token.
func TestLoad_NothingSet(t *testing.T) {
	os.Unsetenv("SILKSTRAND_AGENT_ID")
	os.Unsetenv("SILKSTRAND_AGENT_KEY")
	os.Unsetenv("SILKSTRAND_INSTALL_TOKEN")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when nothing is set")
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLogLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
