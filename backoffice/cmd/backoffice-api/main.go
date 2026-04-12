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
	"golang.org/x/crypto/bcrypt"

	"github.com/jtb75/silkstrand/backoffice/internal/clerkclient"
	"github.com/jtb75/silkstrand/backoffice/internal/config"
	"github.com/jtb75/silkstrand/backoffice/internal/crypto"
	"github.com/jtb75/silkstrand/backoffice/internal/dcclient"
	"github.com/jtb75/silkstrand/backoffice/internal/handler"
	"github.com/jtb75/silkstrand/backoffice/internal/middleware"
	"github.com/jtb75/silkstrand/backoffice/internal/model"
	"github.com/jtb75/silkstrand/backoffice/internal/store"
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

	// Bootstrap admin user if configured and no admins exist
	if err := bootstrapAdmin(context.Background(), pgStore, cfg); err != nil {
		return fmt.Errorf("bootstrapping admin: %w", err)
	}

	// DC and Clerk clients
	dcClient := dcclient.New()
	clerkClient := clerkclient.New(cfg.ClerkSecretKey)
	if cfg.ClerkSecretKey != "" {
		slog.Info("Clerk integration enabled")
	} else {
		slog.Info("Clerk integration disabled (no CLERK_SECRET_KEY)")
	}

	// Handlers
	healthH := handler.NewHealthHandler(pgStore, dcClient, cfg.EncryptionKey)
	dcH := handler.NewDataCenterHandler(pgStore, dcClient, cfg.EncryptionKey)
	tenantH := handler.NewTenantHandler(pgStore, dcClient, clerkClient, cfg.EncryptionKey)
	authH := handler.NewAuthHandler(pgStore, cfg.JWTSecret)

	// Router
	mux := http.NewServeMux()

	// Public routes (no auth)
	mux.HandleFunc("GET /healthz", healthH.Healthz)
	mux.HandleFunc("GET /readyz", healthH.Readyz)
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)

	// Authenticated API routes
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/v1/dashboard", healthH.Dashboard)

	apiMux.HandleFunc("GET /api/v1/data-centers", dcH.List)
	apiMux.HandleFunc("POST /api/v1/data-centers", dcH.Create)
	apiMux.HandleFunc("GET /api/v1/data-centers/{id}", dcH.Get)
	apiMux.HandleFunc("PUT /api/v1/data-centers/{id}", dcH.Update)
	apiMux.HandleFunc("DELETE /api/v1/data-centers/{id}", dcH.Delete)

	apiMux.HandleFunc("GET /api/v1/tenants", tenantH.List)
	apiMux.HandleFunc("POST /api/v1/tenants", tenantH.Create)
	apiMux.HandleFunc("GET /api/v1/tenants/{id}", tenantH.Get)
	apiMux.HandleFunc("PUT /api/v1/tenants/{id}", tenantH.Update)
	apiMux.HandleFunc("PUT /api/v1/tenants/{id}/status", tenantH.UpdateStatus)
	apiMux.HandleFunc("POST /api/v1/tenants/{id}/retry", tenantH.Retry)
	apiMux.HandleFunc("DELETE /api/v1/tenants/{id}", tenantH.Delete)

	// Apply auth middleware to API routes
	authedAPI := middleware.Auth(cfg.JWTSecret)(apiMux)
	mux.Handle("/api/", authedAPI)

	// Apply logging to all routes
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      middleware.Logging(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start health poller
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go healthPoller(ctx, pgStore, dcClient, cfg.EncryptionKey)

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

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	return server.Shutdown(shutdownCtx)
}

func healthPoller(ctx context.Context, s store.Store, dc *dcclient.Client, encKey []byte) {
	if len(encKey) == 0 {
		slog.Warn("encryption key not set, health poller disabled")
		return
	}

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Run immediately on startup
	pollDataCenters(ctx, s, dc, encKey)

	for {
		select {
		case <-ctx.Done():
			slog.Info("health poller stopped")
			return
		case <-ticker.C:
			pollDataCenters(ctx, s, dc, encKey)
		}
	}
}

func pollDataCenters(ctx context.Context, s store.Store, dc *dcclient.Client, encKey []byte) {
	dcs, err := s.ListDataCenters(ctx)
	if err != nil {
		slog.Error("listing data centers for health poll", "error", err)
		return
	}

	for _, dcRecord := range dcs {
		if dcRecord.Status != model.DCStatusActive {
			continue
		}

		apiKey, err := decryptAPIKey(dcRecord.APIKeyEncrypted, encKey)
		if err != nil {
			slog.Error("decrypting API key for health poll", "dc_id", dcRecord.ID, "error", err)
			if err := s.UpdateDataCenterHealth(ctx, dcRecord.ID, "error"); err != nil {
				slog.Error("updating DC health status", "dc_id", dcRecord.ID, "error", err)
			}
			continue
		}

		conn := dcclient.DCConn{APIURL: dcRecord.APIURL, APIKey: string(apiKey)}
		status := "healthy"
		if err := dc.HealthCheck(conn); err != nil {
			slog.Warn("DC health check failed", "dc_id", dcRecord.ID, "error", err)
			status = "unhealthy"
		}

		if err := s.UpdateDataCenterHealth(ctx, dcRecord.ID, status); err != nil {
			slog.Error("updating DC health status", "dc_id", dcRecord.ID, "error", err)
		}
	}
}

func decryptAPIKey(encrypted []byte, key []byte) ([]byte, error) {
	return crypto.Decrypt(encrypted, key)
}

// bootstrapAdmin creates an initial super_admin user on first startup.
// Only runs when BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD are set
// AND no admin users exist yet. After the first admin is created, subsequent
// runs are no-ops even if the env vars are still set.
func bootstrapAdmin(ctx context.Context, s store.Store, cfg *config.Config) error {
	if cfg.BootstrapAdminEmail == "" || cfg.BootstrapAdminPassword == "" {
		return nil
	}

	count, err := s.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("counting admins: %w", err)
	}
	if count > 0 {
		slog.Info("bootstrap admin skipped (admin users already exist)", "count", count)
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.BootstrapAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing bootstrap password: %w", err)
	}

	admin, err := s.CreateAdmin(ctx, cfg.BootstrapAdminEmail, string(hash), "super_admin")
	if err != nil {
		return fmt.Errorf("creating bootstrap admin: %w", err)
	}

	slog.Warn("bootstrap admin created — REMOVE BOOTSTRAP_ADMIN_* env vars now and change the password",
		"email", admin.Email, "role", admin.Role)
	return nil
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
