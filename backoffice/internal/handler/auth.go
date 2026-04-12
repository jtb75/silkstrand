package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jtb75/silkstrand/backoffice/internal/middleware"
	"github.com/jtb75/silkstrand/backoffice/internal/model"
	"github.com/jtb75/silkstrand/backoffice/internal/store"
)

type AuthHandler struct {
	store     store.Store
	jwtSecret string
}

func NewAuthHandler(s store.Store, jwtSecret string) *AuthHandler {
	return &AuthHandler{store: s, jwtSecret: jwtSecret}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	admin, err := h.store.GetAdminByEmail(r.Context(), req.Email)
	if err != nil {
		slog.Error("looking up admin", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if admin == nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := middleware.CreateAdminJWT(h.jwtSecret, admin.ID, admin.Email, admin.Role, 24*time.Hour)
	if err != nil {
		slog.Error("creating JWT", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	writeJSON(w, http.StatusOK, model.LoginResponse{
		Token: token,
		Admin: *admin,
	})
}
