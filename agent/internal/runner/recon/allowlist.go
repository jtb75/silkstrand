package recon

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Allowlist is the customer-controlled scan policy file (ADR 003 D11).
// SaaS cannot override; this is the ultimate authority on what the
// agent will scan.
type Allowlist struct {
	Allow        []string `yaml:"allow"`           // CIDR | IP | "a-b" range | "host" | "*.example.com"
	Deny         []string `yaml:"deny,omitempty"`
	RateLimitPPS int      `yaml:"rate_limit_pps,omitempty"` // clamped to MaxGlobalPPS

	// parsed forms
	allowNets  []*net.IPNet
	allowIPs   []net.IP
	allowRangs []ipRange
	allowHosts []string // exact (lowercase) or "*.suffix"
	denyNets   []*net.IPNet
	denyIPs    []net.IP
	denyRangs  []ipRange
}

type ipRange struct{ from, to net.IP }

var (
	allowlistPath = func() string {
		if v := os.Getenv("SILKSTRAND_SCAN_ALLOWLIST_PATH"); v != "" {
			return v
		}
		return "/etc/silkstrand/scan-allowlist.yaml"
	}()
	allowlistCacheMu sync.Mutex
	allowlistCache   *Allowlist
	allowlistCacheAt time.Time
)

// Load reads the allowlist file, caching by mtime. Fail-closed: a parse
// error returns an error so the calling directive is rejected (better
// than silently scanning whatever the SaaS sent).
func Load() (*Allowlist, error) {
	allowlistCacheMu.Lock()
	defer allowlistCacheMu.Unlock()

	st, err := os.Stat(allowlistPath)
	if err != nil {
		return nil, fmt.Errorf("reading allowlist %s: %w", allowlistPath, err)
	}
	if allowlistCache != nil && st.ModTime().Equal(allowlistCacheAt) {
		return allowlistCache, nil
	}
	raw, err := os.ReadFile(allowlistPath)
	if err != nil {
		return nil, fmt.Errorf("reading allowlist: %w", err)
	}
	var a Allowlist
	if err := yaml.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("parsing allowlist yaml: %w", err)
	}
	if err := a.parse(); err != nil {
		return nil, fmt.Errorf("validating allowlist: %w", err)
	}
	allowlistCache = &a
	allowlistCacheAt = st.ModTime()
	return &a, nil
}

func (a *Allowlist) parse() error {
	for _, e := range a.Allow {
		n, ip, r, host, err := parseEntry(e)
		if err != nil {
			return err
		}
		switch {
		case n != nil:
			a.allowNets = append(a.allowNets, n)
		case ip != nil:
			a.allowIPs = append(a.allowIPs, ip)
		case r != nil:
			a.allowRangs = append(a.allowRangs, *r)
		case host != "":
			a.allowHosts = append(a.allowHosts, host)
		}
	}
	for _, e := range a.Deny {
		n, ip, r, _, err := parseEntry(e)
		if err != nil {
			return err
		}
		switch {
		case n != nil:
			a.denyNets = append(a.denyNets, n)
		case ip != nil:
			a.denyIPs = append(a.denyIPs, ip)
		case r != nil:
			a.denyRangs = append(a.denyRangs, *r)
		}
	}
	return nil
}

func parseEntry(s string) (*net.IPNet, net.IP, *ipRange, string, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("invalid CIDR %q: %w", s, err)
		}
		return n, nil, nil, "", nil
	}
	if i := strings.Index(s, "-"); i > 0 && net.ParseIP(s[:i]) != nil {
		from := net.ParseIP(s[:i])
		to := net.ParseIP(strings.TrimSpace(s[i+1:]))
		if from == nil || to == nil {
			return nil, nil, nil, "", fmt.Errorf("invalid IP range %q", s)
		}
		return nil, nil, &ipRange{from: from, to: to}, "", nil
	}
	if ip := net.ParseIP(s); ip != nil {
		return nil, ip, nil, "", nil
	}
	// Hostname (literal or "*.suffix").
	return nil, nil, nil, strings.ToLower(s), nil
}

// Allows reports whether `ipOrHost` is permitted to scan. For
// hostnames, the caller should also resolve to IPs and call Allows on
// each — both must pass.
func (a *Allowlist) Allows(target string) bool {
	target = strings.TrimSpace(target)
	if ip := net.ParseIP(target); ip != nil {
		return a.allowsIP(ip) && !a.deniesIP(ip)
	}
	host := strings.ToLower(target)
	for _, h := range a.allowHosts {
		if hostMatches(h, host) {
			return true
		}
	}
	return false
}

// AllowsCIDR returns true only if every IP in the CIDR is permitted
// (used pre-scan to vet a directive's target_identifier).
func (a *Allowlist) AllowsCIDR(cidr string) bool {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	// Every network must be fully covered by some allowNet, and not
	// overlap any deny entry.
	covered := false
	for _, an := range a.allowNets {
		if cidrContainsCIDR(an, n) {
			covered = true
			break
		}
	}
	if !covered {
		return false
	}
	for _, dn := range a.denyNets {
		if cidrsOverlap(dn, n) {
			return false
		}
	}
	return true
}

func (a *Allowlist) allowsIP(ip net.IP) bool {
	for _, n := range a.allowNets {
		if n.Contains(ip) {
			return true
		}
	}
	for _, p := range a.allowIPs {
		if p.Equal(ip) {
			return true
		}
	}
	for _, r := range a.allowRangs {
		if ipBetween(ip, r.from, r.to) {
			return true
		}
	}
	return false
}

func (a *Allowlist) deniesIP(ip net.IP) bool {
	for _, n := range a.denyNets {
		if n.Contains(ip) {
			return true
		}
	}
	for _, p := range a.denyIPs {
		if p.Equal(ip) {
			return true
		}
	}
	for _, r := range a.denyRangs {
		if ipBetween(ip, r.from, r.to) {
			return true
		}
	}
	return false
}

func hostMatches(pattern, host string) bool {
	if pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
			return true
		}
	}
	return false
}

func ipBetween(ip, from, to net.IP) bool {
	ip4 := ip.To4()
	from4 := from.To4()
	to4 := to.To4()
	if ip4 == nil || from4 == nil || to4 == nil {
		return false // v6 ranges out of R1a scope
	}
	return compareIPs(ip4, from4) >= 0 && compareIPs(ip4, to4) <= 0
}

func compareIPs(a, b net.IP) int {
	for i := 0; i < len(a); i++ {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

// cidrContainsCIDR is true when outer fully contains inner.
func cidrContainsCIDR(outer, inner *net.IPNet) bool {
	first := inner.IP
	last := lastIP(inner)
	return outer.Contains(first) && outer.Contains(last)
}

func cidrsOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

func lastIP(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP))
	for i := range n.IP {
		ip[i] = n.IP[i] | ^n.Mask[i]
	}
	return ip
}

// EffectivePPS clamps a requested rate against the allowlist setting
// and the global cap.
func (a *Allowlist) EffectivePPS(requested int) int {
	cap := MaxGlobalPPS
	if a.RateLimitPPS > 0 && a.RateLimitPPS < cap {
		cap = a.RateLimitPPS
	}
	if requested > 0 && requested < cap {
		return requested
	}
	return cap
}
