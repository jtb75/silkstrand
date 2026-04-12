package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jtb75/silkstrand/agent/internal/cache"
	"github.com/jtb75/silkstrand/agent/internal/config"
	"github.com/jtb75/silkstrand/agent/internal/runner"
	"github.com/jtb75/silkstrand/agent/internal/tunnel"
)

// version is set via ldflags: -X main.version=$VERSION
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	// Set up structured JSON logging
	logLevel := config.ParseLogLevel(cfg.LogLevel)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	slog.Info("starting silkstrand-agent",
		"version", version,
		"agent_id", cfg.AgentID,
		"api_url", cfg.APIURL,
		"bundle_dir", cfg.BundleDir,
	)

	// Load optional Ed25519 public key for bundle signature verification
	var publicKey ed25519.PublicKey
	if cfg.PublicKeyPath != "" {
		keyData, err := os.ReadFile(cfg.PublicKeyPath)
		if err != nil {
			slog.Error("reading public key", "path", cfg.PublicKeyPath, "error", err)
			os.Exit(1)
		}
		if len(keyData) != ed25519.PublicKeySize {
			slog.Error("invalid public key size", "expected", ed25519.PublicKeySize, "got", len(keyData))
			os.Exit(1)
		}
		publicKey = ed25519.PublicKey(keyData)
		slog.Info("bundle signature verification enabled")
	} else {
		slog.Warn("bundle signature verification disabled (no SILKSTRAND_PUBLIC_KEY set)")
	}

	// Build components
	bundleCache := cache.New(cfg.BundleDir, publicKey)
	pythonRunner := runner.NewPythonRunner()
	tun := tunnel.New(cfg.APIURL, cfg.AgentID, cfg.AgentKey)

	// Concurrency limiter: 1 scan at a time
	scanSem := make(chan struct{}, 1)

	// Wire up directive handler
	tun.OnDirective = func(d tunnel.DirectivePayload) {
		select {
		case scanSem <- struct{}{}:
		default:
			slog.Warn("scan rejected, already running", "scan_id", d.ScanID)
			sendError(tun, d.ScanID, "agent busy: another scan is in progress")
			return
		}

		go func() {
			defer func() { <-scanSem }()
			handleDirective(tun, bundleCache, pythonRunner, d)
		}()
	}

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	tun.Run(ctx, version)
	slog.Info("agent stopped")
}

func handleDirective(tun *tunnel.Tunnel, c *cache.Cache, r runner.Runner, d tunnel.DirectivePayload) {
	slog.Info("received scan directive",
		"scan_id", d.ScanID,
		"bundle", d.BundleName,
		"version", d.BundleVersion,
		"target", d.TargetIdentifier,
	)

	// Notify server that scan has started
	sendStarted(tun, d.ScanID)

	// Resolve bundle from cache
	bundlePath, err := c.Get(d.BundleName, d.BundleVersion)
	if err != nil {
		if errors.Is(err, cache.ErrNotCached) {
			slog.Error("bundle not found in cache", "bundle", d.BundleName, "version", d.BundleVersion)
			sendError(tun, d.ScanID, "bundle not cached: "+d.BundleName+"@"+d.BundleVersion)
		} else {
			slog.Error("cache lookup failed", "error", err)
			sendError(tun, d.ScanID, "cache error: "+err.Error())
		}
		return
	}

	// Load manifest
	manifest, err := runner.LoadManifest(bundlePath)
	if err != nil {
		slog.Error("loading manifest", "error", err)
		sendError(tun, d.ScanID, "manifest error: "+err.Error())
		return
	}

	// Execute the bundle
	ctx := context.Background()
	results, err := r.Run(ctx, runner.RunRequest{
		BundlePath:   bundlePath,
		Manifest:     manifest,
		TargetConfig: d.TargetConfig,
		Credentials:  d.Credentials,
	})
	if err != nil {
		slog.Error("bundle execution failed", "scan_id", d.ScanID, "error", err)
		sendError(tun, d.ScanID, "execution error: "+err.Error())
		return
	}

	// Send results back
	sendResults(tun, d.ScanID, results)
	slog.Info("scan completed", "scan_id", d.ScanID)
}

func sendStarted(tun *tunnel.Tunnel, scanID string) {
	payload, _ := json.Marshal(tunnel.ScanStartedPayload{ScanID: scanID})
	tun.Send(tunnel.Message{Type: tunnel.TypeScanStarted, Payload: payload})
}

func sendResults(tun *tunnel.Tunnel, scanID string, results json.RawMessage) {
	payload, _ := json.Marshal(tunnel.ScanResultsPayload{ScanID: scanID, Results: results})
	tun.Send(tunnel.Message{Type: tunnel.TypeScanResults, Payload: payload})
}

func sendError(tun *tunnel.Tunnel, scanID, errMsg string) {
	payload, _ := json.Marshal(tunnel.ScanErrorPayload{ScanID: scanID, Error: errMsg})
	tun.Send(tunnel.Message{Type: tunnel.TypeScanError, Payload: payload})
}
