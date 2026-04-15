// Package allowlist is the server-side port of the agent's customer-owned
// scan allowlist (ADR 003 D11). The agent is the ultimate authority; this
// package exists so the API can *display* whether a discovered asset would
// be scanned (AllowlistBadge) and gate UI actions accordingly.
//
// Semantics must stay in sync with agent/internal/runner/recon/allowlist.go.
// See allowlist_test.go for the parity cases.
package allowlist

import (
	"fmt"
	"net"
	"strings"
)

// Allowlist is a parsed, pure-function policy: no file I/O, no caching,
// no yaml. Callers construct it from rules the agent reports over WSS.
type Allowlist struct {
	Allow []string
	Deny  []string

	allowNets  []*net.IPNet
	allowIPs   []net.IP
	allowRangs []ipRange
	allowHosts []string
	denyNets   []*net.IPNet
	denyIPs    []net.IP
	denyRangs  []ipRange
}

type ipRange struct{ from, to net.IP }

// Parse validates and compiles rule strings into an Allowlist.
func Parse(allow, deny []string) (*Allowlist, error) {
	a := &Allowlist{Allow: allow, Deny: deny}
	for _, e := range allow {
		n, ip, r, host, err := parseEntry(e)
		if err != nil {
			return nil, err
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
	for _, e := range deny {
		n, ip, r, _, err := parseEntry(e)
		if err != nil {
			return nil, err
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
	return a, nil
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
	return nil, nil, nil, strings.ToLower(s), nil
}

// Allows reports whether target (IP or hostname) is permitted.
// For hostnames, the caller is responsible for also checking resolved IPs
// if it has them — the agent does this; the server does not do DNS
// (per ADR 003 D11: policy stays in the customer network).
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
		suffix := pattern[1:]
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
		return false
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
