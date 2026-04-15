package allowlist

import "testing"

// Parity with agent/internal/runner/recon/allowlist_test.go.
func TestAllowsBasic(t *testing.T) {
	a, err := Parse(
		[]string{"10.0.0.0/16", "192.168.50.5", "192.168.50.10-192.168.50.50", "*.internal.example.com", "corp-internal.example.com"},
		[]string{"10.0.99.0/24"},
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cases := []struct {
		target string
		want   bool
	}{
		{"10.0.1.5", true},
		{"10.0.99.5", false},
		{"192.168.50.5", true},
		{"192.168.50.20", true},
		{"192.168.50.51", false},
		{"172.16.0.1", false},
		{"corp-internal.example.com", true},
		{"db.internal.example.com", true},
		{"public.example.com", false},
	}
	for _, c := range cases {
		if got := a.Allows(c.target); got != c.want {
			t.Errorf("Allows(%q) = %v, want %v", c.target, got, c.want)
		}
	}
}

func TestEmpty(t *testing.T) {
	a, err := Parse(nil, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if a.Allows("10.0.0.1") {
		t.Error("empty allowlist must not allow anything")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]string{"10.0.0.0/notacidr"}, nil); err == nil {
		t.Error("expected error on bad CIDR")
	}
	if _, err := Parse(nil, []string{"1.2.3.4-notanip"}); err == nil {
		t.Error("expected error on bad range")
	}
}
