package middleware

import (
	"context"
	"net/http"

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

// Tenant extracts the tenant_id from the authenticated claims and injects
// it into the context for use by the store layer.
func Tenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		if claims == nil || claims.TenantID == "" {
			http.Error(w, `{"error":"missing tenant context"}`, http.StatusForbidden)
			return
		}

		ctx := store.WithTenantID(r.Context(), claims.TenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
