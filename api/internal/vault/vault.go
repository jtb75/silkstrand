// Package vault provides a HashiCorp Vault credential resolver for
// ADR 004 C2. The DC API calls Resolve at scan time to fetch
// credentials server-side from a Vault KV v2 secret engine; the agent
// receives plaintext the same way it does for static sources.
package vault

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ResolveConfig holds the per-credential-source configuration stored
// in credential_sources.config JSONB for type=hashicorp_vault.
type ResolveConfig struct {
	VaultURL         string `json:"vault_url"`
	AuthMethod       string `json:"auth_method"`         // "token" for v1
	Token            string `json:"token"`                // used when auth_method=token
	SecretPath       string `json:"secret_path"`          // e.g., "secret/data/mssql-creds"
	SecretKeyUsername string `json:"secret_key_username"`  // JSON key for username in the secret data
	SecretKeyPassword string `json:"secret_key_password"`  // JSON key for password
	Namespace        string `json:"namespace"`             // optional Vault namespace
	TLSSkipVerify    bool   `json:"tls_skip_verify"`       // for dev/self-signed certs
}

// Credential is the resolved username + password extracted from the
// Vault KV v2 secret.
type Credential struct {
	Username string
	Password string
}

// vaultKVv2Response models the Vault KV v2 read response. The actual
// secret data is nested under data.data (double nesting).
type vaultKVv2Response struct {
	Data struct {
		Data map[string]any `json:"data"`
	} `json:"data"`
}

// Resolve fetches a secret from HashiCorp Vault KV v2 and extracts
// username + password from the nested data using the configured keys.
//
// The flow:
//  1. Validate required config fields.
//  2. Build HTTP client (with optional TLS skip verify).
//  3. GET {vault_url}/v1/{secret_path} with X-Vault-Token header.
//  4. Parse the KV v2 response (double-nested data).
//  5. Extract username + password using the configured keys.
func Resolve(ctx context.Context, cfg ResolveConfig) (*Credential, error) {
	if cfg.VaultURL == "" {
		return nil, fmt.Errorf("vault: vault_url is required")
	}
	if cfg.SecretPath == "" {
		return nil, fmt.Errorf("vault: secret_path is required")
	}
	if cfg.AuthMethod == "" {
		cfg.AuthMethod = "token"
	}
	if cfg.AuthMethod != "token" {
		return nil, fmt.Errorf("vault: unsupported auth_method %q (only \"token\" is supported)", cfg.AuthMethod)
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("vault: token is required for auth_method=token")
	}

	// Default key names if not specified.
	usernameKey := cfg.SecretKeyUsername
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := cfg.SecretKeyPassword
	if passwordKey == "" {
		passwordKey = "password"
	}

	// 1. Build HTTP client.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.TLSSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user-configured for dev/self-signed
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// 2. Build the request URL. Vault KV v2 API: GET /v1/{path}
	// The caller provides the full mount+data path (e.g. "secret/data/mssql-creds").
	url := strings.TrimRight(cfg.VaultURL, "/") + "/v1/" + strings.TrimLeft(cfg.SecretPath, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("vault: building request: %w", err)
	}
	req.Header.Set("X-Vault-Token", cfg.Token)
	if cfg.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", cfg.Namespace)
	}

	// 3. Execute.
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vault: reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Truncate body for logging.
		msg := string(body)
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		return nil, fmt.Errorf("vault: HTTP %d: %s", resp.StatusCode, msg)
	}

	// 4. Parse the KV v2 response.
	var kvResp vaultKVv2Response
	if err := json.Unmarshal(body, &kvResp); err != nil {
		return nil, fmt.Errorf("vault: parsing response JSON: %w", err)
	}

	if kvResp.Data.Data == nil {
		return nil, fmt.Errorf("vault: secret at %q has no data", cfg.SecretPath)
	}

	// 5. Extract username + password.
	username, ok := kvResp.Data.Data[usernameKey].(string)
	if !ok {
		return nil, fmt.Errorf("vault: key %q not found or not a string in secret data", usernameKey)
	}
	password, ok := kvResp.Data.Data[passwordKey].(string)
	if !ok {
		return nil, fmt.Errorf("vault: key %q not found or not a string in secret data", passwordKey)
	}

	return &Credential{
		Username: username,
		Password: password,
	}, nil
}
