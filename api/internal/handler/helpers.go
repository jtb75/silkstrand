package handler

import (
	"encoding/json"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// claimsActorID extracts the user identifier from JWT claims. Prefers
// Sub (new layout) over UserID (legacy dev JWT).
func claimsActorID(c *middleware.Claims) string {
	if c == nil {
		return ""
	}
	if c.Sub != "" {
		return c.Sub
	}
	return c.UserID
}
