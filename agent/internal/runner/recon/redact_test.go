package recon

import (
	"strings"
	"testing"
)

func TestRedactStringPatterns(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // substring that must NOT be present
	}{
		{"aws_key", "leak: AKIAIOSFODNN7EXAMPLE in body", "AKIAIOSFODNN7EXAMPLE"},
		{"jwt", "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTYifQ.AbCdEfGhIjKlMnOpQr", "eyJhbGci"},
		{"bearer", "X-Auth: Bearer abcdefghijklmnopqrstuv", "abcdefghijklmnopqrstuv"},
		{"password_kv", "config: password=hunter2", "hunter2"},
		{"pg_url", "DSN: postgres://user:hunter2@db.local:5432/x", "user:hunter2@"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := String(c.in)
			if strings.Contains(out, c.want) {
				t.Errorf("redacted output still contained %q\nin:  %s\nout: %s", c.want, c.in, out)
			}
			if !strings.Contains(out, redactionMarker) {
				t.Errorf("expected redaction marker, got %q", out)
			}
		})
	}
}

func TestRedactStringTruncates(t *testing.T) {
	t.Setenv("SILKSTRAND_EVIDENCE_BODY_MAX", "16")
	in := "this is a long body that exceeds the cap"
	out := String(in)
	if !strings.HasSuffix(out, "...[truncated]") {
		t.Errorf("expected truncation suffix, got %q", out)
	}
}
