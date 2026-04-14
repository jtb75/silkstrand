package recon

import "testing"

func TestAllowsBasic(t *testing.T) {
	a := &Allowlist{
		Allow: []string{"10.0.0.0/16", "192.168.50.5", "192.168.50.10-192.168.50.50", "*.internal.example.com", "corp-internal.example.com"},
		Deny:  []string{"10.0.99.0/24"},
	}
	if err := a.parse(); err != nil {
		t.Fatalf("parse: %v", err)
	}

	cases := []struct {
		target string
		want   bool
	}{
		{"10.0.1.5", true},
		{"10.0.99.5", false}, // denied
		{"192.168.50.5", true},
		{"192.168.50.20", true},  // in range
		{"192.168.50.51", false}, // outside range
		{"172.16.0.1", false},
		{"corp-internal.example.com", true},
		{"db.internal.example.com", true},  // wildcard
		{"public.example.com", false},      // not allowed
	}
	for _, c := range cases {
		if got := a.Allows(c.target); got != c.want {
			t.Errorf("Allows(%q) = %v, want %v", c.target, got, c.want)
		}
	}
}

func TestAllowsCIDR(t *testing.T) {
	a := &Allowlist{
		Allow: []string{"10.0.0.0/8"},
		Deny:  []string{"10.0.99.0/24"},
	}
	if err := a.parse(); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !a.AllowsCIDR("10.1.0.0/16") {
		t.Error("10.1.0.0/16 should be allowed")
	}
	if a.AllowsCIDR("10.0.99.0/24") {
		t.Error("10.0.99.0/24 should be denied")
	}
	if a.AllowsCIDR("11.0.0.0/16") {
		t.Error("11.0.0.0/16 should not be allowed")
	}
}

func TestEffectivePPS(t *testing.T) {
	a := &Allowlist{RateLimitPPS: 500}
	if got := a.EffectivePPS(0); got != 500 {
		t.Errorf("default rate: got %d, want 500", got)
	}
	if got := a.EffectivePPS(200); got != 200 {
		t.Errorf("respect requested: got %d, want 200", got)
	}
	if got := a.EffectivePPS(10000); got != 500 {
		t.Errorf("clamp to allowlist: got %d, want 500", got)
	}

	noLimit := &Allowlist{}
	if got := noLimit.EffectivePPS(10000); got != MaxGlobalPPS {
		t.Errorf("clamp to global: got %d, want %d", got, MaxGlobalPPS)
	}
}
