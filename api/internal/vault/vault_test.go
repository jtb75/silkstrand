package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolve_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "test-token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path != "/v1/secret/data/mssql-creds" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resp := map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"username": "sa",
					"password": "S3cret!",
				},
				"metadata": map[string]any{
					"version": 1,
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cred, err := Resolve(context.Background(), ResolveConfig{
		VaultURL:         srv.URL,
		AuthMethod:       "token",
		Token:            "test-token",
		SecretPath:       "secret/data/mssql-creds",
		SecretKeyUsername: "username",
		SecretKeyPassword: "password",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Username != "sa" {
		t.Errorf("username = %q, want %q", cred.Username, "sa")
	}
	if cred.Password != "S3cret!" {
		t.Errorf("password = %q, want %q", cred.Password, "S3cret!")
	}
}

func TestResolve_CustomKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"db_user": "admin",
					"db_pass": "p@ss",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cred, err := Resolve(context.Background(), ResolveConfig{
		VaultURL:         srv.URL,
		Token:            "tok",
		SecretPath:       "secret/data/db",
		SecretKeyUsername: "db_user",
		SecretKeyPassword: "db_pass",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Username != "admin" || cred.Password != "p@ss" {
		t.Errorf("got %q / %q", cred.Username, cred.Password)
	}
}

func TestResolve_Namespace(t *testing.T) {
	var gotNamespace string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNamespace = r.Header.Get("X-Vault-Namespace")
		resp := map[string]any{
			"data": map[string]any{
				"data": map[string]any{"username": "u", "password": "p"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	_, err := Resolve(context.Background(), ResolveConfig{
		VaultURL:   srv.URL,
		Token:      "tok",
		SecretPath: "secret/data/x",
		Namespace:  "engineering",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotNamespace != "engineering" {
		t.Errorf("namespace header = %q, want %q", gotNamespace, "engineering")
	}
}

func TestResolve_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		cfg  ResolveConfig
		want string
	}{
		{"no vault_url", ResolveConfig{Token: "t", SecretPath: "s"}, "vault_url is required"},
		{"no secret_path", ResolveConfig{VaultURL: "http://x", Token: "t"}, "secret_path is required"},
		{"no token", ResolveConfig{VaultURL: "http://x", SecretPath: "s"}, "token is required"},
		{"bad auth", ResolveConfig{VaultURL: "http://x", SecretPath: "s", Token: "t", AuthMethod: "ldap"}, "unsupported auth_method"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(context.Background(), tt.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); !contains(got, tt.want) {
				t.Errorf("error = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestResolve_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errors":["permission denied"]}`))
	}))
	defer srv.Close()

	_, err := Resolve(context.Background(), ResolveConfig{
		VaultURL:   srv.URL,
		Token:      "bad-token",
		SecretPath: "secret/data/x",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "HTTP 403") {
		t.Errorf("error = %q, want HTTP 403", err.Error())
	}
}

func TestResolve_MissingKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"username": "sa",
					// no password key
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	_, err := Resolve(context.Background(), ResolveConfig{
		VaultURL:   srv.URL,
		Token:      "tok",
		SecretPath: "secret/data/x",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "password") {
		t.Errorf("error = %q, want mention of password key", err.Error())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
