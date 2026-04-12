package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimit is a simple in-memory token-bucket rate limiter keyed by client
// IP. Intended for public endpoints that can be abused by brute-force or
// email-enumeration attacks — POST /auth/login, /forgot-password, etc.
//
// Each key (IP) gets `burst` tokens, refilling at 1 per `refill` interval.
// Exceeding the burst returns 429 Too Many Requests.
//
// This is best-effort protection only: Cloud Run autoscales across
// instances, so an attacker hitting a single endpoint will be distributed
// across pods. For real protection, front with Cloud Armor. For dev and
// MVP-scale traffic, in-memory is enough to slow down casual abuse.
type rateLimiter struct {
	burst  int
	refill time.Duration

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens int
	last   time.Time
}

func RateLimit(burst int, refill time.Duration) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		burst:   burst,
		refill:  refill,
		buckets: make(map[string]*bucket),
	}
	// Cleanup goroutine — drop idle buckets every 5 minutes.
	go rl.cleanup()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rl.allow(clientIP(r)) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Retry-After", "60")
			writeJSONErr(w, http.StatusTooManyRequests, "too many requests; try again shortly")
		})
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &bucket{tokens: rl.burst - 1, last: now}
		return true
	}
	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.last)
	refilled := int(elapsed / rl.refill)
	if refilled > 0 {
		b.tokens = min(rl.burst, b.tokens+refilled)
		b.last = b.last.Add(time.Duration(refilled) * rl.refill)
	}
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-30 * time.Minute)
		rl.mu.Lock()
		for k, b := range rl.buckets {
			if b.last.Before(cutoff) {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// clientIP extracts the caller's IP, preferring the first hop in
// X-Forwarded-For (Cloud Run sets this) and falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
