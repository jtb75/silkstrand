package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

type claimsKey struct{}

// SetClaims stores auth claims in the context.
func SetClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

// GetClaims retrieves auth claims from the context.
func GetClaims(ctx context.Context) *Claims {
	v, _ := ctx.Value(claimsKey{}).(*Claims)
	return v
}

// tenantCache provides a short-TTL cache for tenant status lookups.
type tenantCache struct {
	mu      sync.RWMutex
	entries map[string]tenantCacheEntry
}

type tenantCacheEntry struct {
	status    string
	expiresAt time.Time
}

const tenantCacheTTL = 5 * time.Second

func newTenantCache() *tenantCache {
	return &tenantCache{entries: make(map[string]tenantCacheEntry)}
}

func (c *tenantCache) get(id string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[id]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.status, true
}

func (c *tenantCache) set(id, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = tenantCacheEntry{
		status:    status,
		expiresAt: time.Now().Add(tenantCacheTTL),
	}
}

// Tenant extracts the tenant_id from the authenticated claims, checks that
// the tenant is active, and injects the tenant ID into the context.
func Tenant(s store.Store) func(http.Handler) http.Handler {
	cache := newTenantCache()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil || claims.TenantID == "" {
				http.Error(w, `{"error":"missing tenant context"}`, http.StatusForbidden)
				return
			}

			// Check tenant status (cached)
			status, ok := cache.get(claims.TenantID)
			if !ok {
				tenant, err := s.GetTenantByID(r.Context(), claims.TenantID)
				if err != nil {
					http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
					return
				}
				if tenant == nil {
					http.Error(w, `{"error":"tenant not found"}`, http.StatusForbidden)
					return
				}
				status = tenant.Status
				cache.set(claims.TenantID, status)
			}

			if status != model.TenantStatusActive {
				http.Error(w, `{"error":"tenant suspended"}`, http.StatusForbidden)
				return
			}

			ctx := store.WithTenantID(r.Context(), claims.TenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
