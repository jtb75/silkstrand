package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/jtb75/silkstrand/api/internal/config"
	"github.com/jtb75/silkstrand/api/internal/handler"
	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Database
	pgStore, err := store.NewPostgresStore(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pgStore.Close()

	// Run migrations
	if err := runMigrations(pgStore.DB()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Redis pub/sub
	ps, err := pubsub.New(cfg.RedisURL)
	if err != nil {
		slog.Warn("redis not available, pub/sub disabled", "error", err)
		ps = nil
	} else {
		defer ps.Close()
	}

	// WebSocket hub
	hub := websocket.NewHub()

	// Handlers
	healthH := handler.NewHealthHandler(pgStore, redisPingFunc(ps))
	targetH := handler.NewTargetHandler(pgStore)
	scanH := handler.NewScanHandler(pgStore)
	agentH := handler.NewAgentHandler(hub)

	// Router
	mux := http.NewServeMux()

	// Public routes (no auth)
	mux.HandleFunc("GET /healthz", healthH.Healthz)
	mux.HandleFunc("GET /readyz", healthH.Readyz)

	// Agent WebSocket (agent auth — separate from user auth)
	mux.HandleFunc("GET /ws/agent", agentH.Connect)

	// Authenticated API routes
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/v1/targets", targetH.List)
	apiMux.HandleFunc("POST /api/v1/targets", targetH.Create)
	apiMux.HandleFunc("GET /api/v1/targets/{id}", targetH.Get)
	apiMux.HandleFunc("PUT /api/v1/targets/{id}", targetH.Update)
	apiMux.HandleFunc("DELETE /api/v1/targets/{id}", targetH.Delete)
	apiMux.HandleFunc("POST /api/v1/scans", scanH.Create)
	apiMux.HandleFunc("GET /api/v1/scans", scanH.List)
	apiMux.HandleFunc("GET /api/v1/scans/{id}", scanH.Get)

	// Apply auth + tenant middleware to API routes
	authedAPI := middleware.Auth(cfg.JWTSecret)(middleware.Tenant(apiMux))
	mux.Handle("/api/", authedAPI)

	// Apply logging to all routes
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      middleware.Logging(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "port", cfg.Port)
		errCh <- server.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return server.Shutdown(ctx)
}

func runMigrations(db *sql.DB) error {
	sourceDriver, err := iofs.New(store.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("creating migration source: %w", err)
	}

	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("creating migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	slog.Info("migrations complete")
	return nil
}

func redisPingFunc(ps *pubsub.PubSub) func(context.Context) error {
	if ps == nil {
		return nil
	}
	return ps.Ping
}
