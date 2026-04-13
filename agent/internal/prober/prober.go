// Package prober performs lightweight connectivity checks against a target
// independent of the scan bundle pipeline. Used by the "Test connection"
// button in the tenant UI.
//
// Each supported target type has its own driver path; the dispatch lives
// in Probe(). Adding a new target type means: vendor a pure-Go driver,
// add a probe<Tech>() function, and add a case to the switch.
package prober

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	// register the "sqlserver" database/sql driver
	_ "github.com/microsoft/go-mssqldb"
)

// DatabaseCredentials mirrors what the UI credential form saves. All
// supported DB engines use the same {username, password} shape.
type DatabaseCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Result is a compact outcome for the UI.
type Result struct {
	OK     bool
	Error  string
	Detail string // e.g. "PostgreSQL 16.13 …"
}

const probeTimeout = 10 * time.Second

// Probe dispatches on target type. Times out after 10s regardless.
func Probe(ctx context.Context, targetType string, configRaw, credsRaw json.RawMessage) Result {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	creds, err := parseCreds(credsRaw)
	if err != nil {
		return Result{OK: false, Error: err.Error()}
	}

	switch targetType {
	case "postgresql", "aurora_postgresql":
		return probePostgres(ctx, configRaw, creds)
	case "mssql":
		return probeMSSQL(ctx, configRaw, creds)
	case "mongodb":
		return probeMongoDB(ctx, configRaw, creds)
	case "mysql", "aurora_mysql":
		return probeMySQL(ctx, configRaw, creds)
	// Legacy value — kept for the brief window between rolling out the
	// type rename migration and seeing all rows updated. Treat as Postgres.
	case "database":
		return probePostgres(ctx, configRaw, creds)
	default:
		return Result{OK: false,
			Error: fmt.Sprintf("connectivity test not implemented for target type %q", targetType)}
	}
}

func parseCreds(raw json.RawMessage) (DatabaseCredentials, error) {
	var creds DatabaseCredentials
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &creds); err != nil {
			return creds, fmt.Errorf("invalid credentials: %w", err)
		}
	}
	if creds.Username == "" {
		return creds, fmt.Errorf("no credential set for this target (Targets → Credential)")
	}
	return creds, nil
}

// ---------------------------------------------------------------------------
// PostgreSQL
// ---------------------------------------------------------------------------

type postgresConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	SSLMode  string `json:"sslmode"`
}

func probePostgres(ctx context.Context, raw json.RawMessage, creds DatabaseCredentials) Result {
	var cfg postgresConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Result{OK: false, Error: "invalid target config: " + err.Error()}
	}
	if cfg.Host == "" {
		return Result{OK: false, Error: "target config missing 'host'"}
	}
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "prefer"
	}
	if cfg.Database == "" {
		cfg.Database = "postgres"
	}

	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s connect_timeout=8",
		cfg.Host, cfg.Port, cfg.Database, creds.Username, creds.Password, cfg.SSLMode)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return Result{OK: false, Error: err.Error()}
	}
	defer conn.Close(ctx)

	var version string
	if err := conn.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		return Result{OK: false, Error: "connected but version query failed: " + err.Error()}
	}
	return Result{OK: true, Detail: version}
}

// ---------------------------------------------------------------------------
// SQL Server (MSSQL)
// ---------------------------------------------------------------------------

type mssqlConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Encrypt  bool   `json:"encrypt"`
}

func probeMSSQL(ctx context.Context, raw json.RawMessage, creds DatabaseCredentials) Result {
	var cfg mssqlConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Result{OK: false, Error: "invalid target config: " + err.Error()}
	}
	if cfg.Host == "" {
		return Result{OK: false, Error: "target config missing 'host'"}
	}
	if cfg.Port == 0 {
		cfg.Port = 1433
	}
	if cfg.Database == "" {
		cfg.Database = "master"
	}

	// go-mssqldb URL form. encrypt=disable / true; trust the cert by
	// default since most internal SQL Servers use self-signed certs.
	encrypt := "disable"
	if cfg.Encrypt {
		encrypt = "true"
	}
	u := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(creds.Username, creds.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
	}
	q := u.Query()
	q.Set("database", cfg.Database)
	q.Set("encrypt", encrypt)
	q.Set("TrustServerCertificate", "true")
	q.Set("connection timeout", "8")
	u.RawQuery = q.Encode()

	db, err := sql.Open("sqlserver", u.String())
	if err != nil {
		return Result{OK: false, Error: err.Error()}
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return Result{OK: false, Error: err.Error()}
	}

	var version string
	if err := db.QueryRowContext(ctx, "SELECT @@VERSION").Scan(&version); err != nil {
		return Result{OK: false, Error: "connected but version query failed: " + err.Error()}
	}
	// @@VERSION returns a multi-line banner; first line is enough.
	if i := indexNL(version); i > 0 {
		version = version[:i]
	}
	return Result{OK: true, Detail: version}
}

func indexNL(s string) int {
	for i, c := range s {
		if c == '\n' || c == '\r' {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// MongoDB
// ---------------------------------------------------------------------------

type mongoConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	AuthSource string `json:"auth_source"`
	TLS        bool   `json:"tls"`
}

func probeMongoDB(ctx context.Context, raw json.RawMessage, creds DatabaseCredentials) Result {
	var cfg mongoConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Result{OK: false, Error: "invalid target config: " + err.Error()}
	}
	if cfg.Host == "" {
		return Result{OK: false, Error: "target config missing 'host'"}
	}
	if cfg.Port == 0 {
		cfg.Port = 27017
	}
	if cfg.AuthSource == "" {
		cfg.AuthSource = "admin"
	}

	scheme := "mongodb"
	uri := fmt.Sprintf("%s://%s:%s@%s:%d/?authSource=%s",
		scheme,
		url.QueryEscape(creds.Username), url.QueryEscape(creds.Password),
		cfg.Host, cfg.Port, cfg.AuthSource)

	clientOpts := options.Client().ApplyURI(uri).SetServerSelectionTimeout(8 * time.Second)
	if cfg.TLS {
		clientOpts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // self-signed common in dev
	}

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return Result{OK: false, Error: err.Error()}
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return Result{OK: false, Error: err.Error()}
	}

	// Pull buildInfo for a useful Detail string.
	var bi struct {
		Version string `bson:"version"`
	}
	if err := client.Database("admin").RunCommand(ctx,
		map[string]any{"buildInfo": 1}).Decode(&bi); err == nil && bi.Version != "" {
		return Result{OK: true, Detail: "MongoDB " + bi.Version}
	}
	return Result{OK: true, Detail: "MongoDB (version unknown)"}
}

// ---------------------------------------------------------------------------
// MySQL / Aurora MySQL
// ---------------------------------------------------------------------------

type mysqlConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
}

func probeMySQL(ctx context.Context, raw json.RawMessage, creds DatabaseCredentials) Result {
	var cfg mysqlConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Result{OK: false, Error: "invalid target config: " + err.Error()}
	}
	if cfg.Host == "" {
		return Result{OK: false, Error: "target config missing 'host'"}
	}
	if cfg.Port == 0 {
		cfg.Port = 3306
	}

	mcfg := mysql.NewConfig()
	mcfg.User = creds.Username
	mcfg.Passwd = creds.Password
	mcfg.Net = "tcp"
	mcfg.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	mcfg.DBName = cfg.Database
	mcfg.Timeout = 8 * time.Second
	mcfg.ReadTimeout = 8 * time.Second

	db, err := sql.Open("mysql", mcfg.FormatDSN())
	if err != nil {
		return Result{OK: false, Error: err.Error()}
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return Result{OK: false, Error: err.Error()}
	}

	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return Result{OK: false, Error: "connected but version query failed: " + err.Error()}
	}
	return Result{OK: true, Detail: "MySQL " + version}
}
