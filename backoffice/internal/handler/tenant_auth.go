package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jtb75/silkstrand/backoffice/internal/crypto"
	"github.com/jtb75/silkstrand/backoffice/internal/mailer"
	"github.com/jtb75/silkstrand/backoffice/internal/middleware"
	"github.com/jtb75/silkstrand/backoffice/internal/model"
	"github.com/jtb75/silkstrand/backoffice/internal/store"
)

const (
	tenantJWTExpiry     = time.Hour
	inviteExpiry        = 7 * 24 * time.Hour
	passwordResetExpiry = time.Hour
)

// TenantAuthHandler handles authentication for tenant end users.
type TenantAuthHandler struct {
	store        store.Store
	mailer       mailer.Mailer
	jwtSecret    string
	tenantWebURL string // Base URL for building invite / reset links in emails
}

func NewTenantAuthHandler(s store.Store, m mailer.Mailer, jwtSecret, tenantWebURL string) *TenantAuthHandler {
	return &TenantAuthHandler{
		store:        s,
		mailer:       m,
		jwtSecret:    jwtSecret,
		tenantWebURL: tenantWebURL,
	}
}

// POST /api/v1/tenant-auth/accept-invite
// Body: {token, password}
// Creates the user if new, sets/replaces password, marks invite accepted,
// creates the tenant membership, returns a JWT.
func (h *TenantAuthHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "token and password are required")
		return
	}

	inv, err := h.store.GetInvitationByTokenHash(r.Context(), crypto.HashToken(req.Token))
	if err != nil {
		slog.Error("getting invitation", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to look up invitation")
		return
	}
	if inv == nil {
		writeError(w, http.StatusNotFound, "invitation not found or invalid")
		return
	}
	if inv.AcceptedAt != nil {
		writeError(w, http.StatusGone, "invitation already accepted")
		return
	}
	if time.Now().After(inv.ExpiresAt) {
		writeError(w, http.StatusGone, "invitation has expired")
		return
	}

	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Upsert user by email, preserving existing password if user already has
	// one for another tenant (they'll log in with that; this invite just
	// grants access — we don't overwrite an established password).
	user, err := h.store.GetUserByEmail(r.Context(), inv.Email)
	if err != nil {
		slog.Error("looking up user by email", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}
	if user == nil {
		user, err = h.store.CreateUser(r.Context(), inv.Email, hash)
		if err != nil {
			slog.Error("creating user", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	} else if user.PasswordHash == "" {
		// Existing user record without a password (invited then never accepted).
		if err := h.store.UpdateUserPassword(r.Context(), user.ID, hash); err != nil {
			slog.Error("setting user password", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to set password")
			return
		}
		user.PasswordHash = hash
	}
	// Invite click proves email ownership.
	if user.EmailVerifiedAt == nil {
		_ = h.store.MarkUserEmailVerified(r.Context(), user.ID)
	}

	// Enforce membership cap.
	count, err := h.store.CountMembershipsByUser(r.Context(), user.ID)
	if err != nil {
		slog.Error("counting memberships", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check memberships")
		return
	}
	existing, err := h.store.GetMembership(r.Context(), user.ID, inv.TenantID)
	if err != nil {
		slog.Error("checking existing membership", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check existing membership")
		return
	}
	if existing == nil && count >= model.MaxMembershipsPerUser {
		writeError(w, http.StatusForbidden, "user has reached the maximum number of tenants")
		return
	}

	if _, err := h.store.CreateMembership(r.Context(), user.ID, inv.TenantID, inv.Role); err != nil {
		slog.Error("creating membership", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create membership")
		return
	}
	if err := h.store.MarkInvitationAccepted(r.Context(), inv.ID); err != nil {
		slog.Error("marking invitation accepted", "error", err)
	}

	h.issueJWTForTenant(w, r, user, inv.TenantID, inv.Role)
}

// POST /api/v1/tenant-auth/login
// Body: {email, password}
// Returns JWT scoped to the user's most recent tenant (first membership).
func (h *TenantAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		slog.Error("login: getting user", "error", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if user == nil || user.PasswordHash == "" {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err := crypto.CheckPassword(user.PasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	memberships, err := h.store.ListMembershipsByUser(r.Context(), user.ID)
	if err != nil {
		slog.Error("login: listing memberships", "error", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if len(memberships) == 0 {
		writeError(w, http.StatusForbidden, "no active tenant memberships")
		return
	}
	_ = h.store.TouchUserLogin(r.Context(), user.ID)

	m := memberships[0]
	h.issueJWTForTenant(w, r, user, m.TenantID, m.Role)
}

// POST /api/v1/tenant-auth/switch-org (authenticated)
// Body: {tenant_id}
// Verifies the authenticated user belongs to tenant_id, issues a new JWT
// scoped to that tenant.
func (h *TenantAuthHandler) SwitchOrg(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetTenantClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id is required")
		return
	}

	membership, err := h.store.GetMembership(r.Context(), claims.Sub, req.TenantID)
	if err != nil {
		slog.Error("switch-org: getting membership", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to switch")
		return
	}
	if membership == nil {
		writeError(w, http.StatusForbidden, "not a member of this tenant")
		return
	}

	user, err := h.store.GetUserByID(r.Context(), claims.Sub)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	h.issueJWTForTenant(w, r, user, req.TenantID, membership.Role)
}

// GET /api/v1/tenant-auth/me (authenticated)
// Returns the user and all active memberships.
func (h *TenantAuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetTenantClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := h.store.GetUserByID(r.Context(), claims.Sub)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	memberships, err := h.store.ListMembershipsByUser(r.Context(), user.ID)
	if err != nil {
		slog.Error("me: listing memberships", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list memberships")
		return
	}
	if memberships == nil {
		memberships = []model.MembershipSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":        user,
		"memberships": memberships,
		"active": map[string]string{
			"tenant_id": claims.TenantID,
			"dc_id":     claims.DCID,
			"role":      claims.Role,
		},
	})
}

// POST /api/v1/tenant-auth/forgot-password
// Body: {email}
// Always returns 204 so attackers can't enumerate registered emails.
func (h *TenantAuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		// Still return 204 — don't leak shape either.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		slog.Error("forgot-password: getting user", "error", err)
		// Don't leak to caller, but log.
	}
	if user != nil {
		plaintext, hash, err := crypto.NewURLToken()
		if err == nil {
			expiry := time.Now().Add(passwordResetExpiry)
			if err := h.store.CreatePasswordReset(r.Context(), user.ID, hash, expiry); err == nil {
				resetURL := h.tenantWebURL + "/reset-password?token=" + plaintext
				if err := h.mailer.SendPasswordReset(user.Email, resetURL); err != nil {
					slog.Warn("sending password reset email", "error", err)
				}
			} else {
				slog.Error("creating password reset", "error", err)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/tenant-auth/reset-password
// Body: {token, password}
func (h *TenantAuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "token and password are required")
		return
	}

	hash := crypto.HashToken(req.Token)
	userID, expiresAt, usedAt, err := h.store.GetPasswordResetByTokenHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "reset link not found or invalid")
			return
		}
		slog.Error("reset-password: looking up token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if usedAt != nil {
		writeError(w, http.StatusGone, "reset link already used")
		return
	}
	if time.Now().After(expiresAt) {
		writeError(w, http.StatusGone, "reset link expired")
		return
	}

	pwHash, err := crypto.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.UpdateUserPassword(r.Context(), userID, pwHash); err != nil {
		slog.Error("reset-password: updating password", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	_ = h.store.MarkPasswordResetUsed(r.Context(), hash)
	w.WriteHeader(http.StatusNoContent)
}

// issueJWTForTenant looks up the DC ID for the tenant, mints a JWT, and
// writes the response.
func (h *TenantAuthHandler) issueJWTForTenant(w http.ResponseWriter, r *http.Request, user *model.User, tenantID, role string) {
	// Look up DC ID for the tenant so we can bake it into the token.
	tenant, err := h.store.GetTenant(r.Context(), tenantID)
	if err != nil || tenant == nil {
		writeError(w, http.StatusInternalServerError, "tenant lookup failed")
		return
	}

	token, err := middleware.CreateTenantJWT(
		h.jwtSecret, user.ID, user.Email, tenantID, tenant.DataCenterID, role, tenantJWTExpiry)
	if err != nil {
		slog.Error("creating tenant JWT", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]string{
			"id":    user.ID,
			"email": user.Email,
		},
		"active": map[string]string{
			"tenant_id":      tenantID,
			"data_center_id": tenant.DataCenterID,
			"role":           role,
		},
	})
}

// GET /api/v1/tenant-auth/members (authenticated)
// Returns the list of users in the caller's active tenant.
func (h *TenantAuthHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetTenantClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	members, err := h.store.ListTenantMembers(r.Context(), claims.TenantID)
	if err != nil {
		slog.Error("listing tenant members", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	if members == nil {
		members = []model.TenantMember{}
	}
	writeJSON(w, http.StatusOK, members)
}

// POST /api/v1/tenant-auth/invites (authenticated, admin-only)
// Body: {email, role}
func (h *TenantAuthHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetTenantClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if claims.Role != model.MembershipRoleAdmin {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Role != model.MembershipRoleAdmin && req.Role != model.MembershipRoleMember {
		writeError(w, http.StatusBadRequest, "role must be 'admin' or 'member'")
		return
	}

	tenant, err := h.store.GetTenant(r.Context(), claims.TenantID)
	if err != nil || tenant == nil {
		writeError(w, http.StatusInternalServerError, "tenant lookup failed")
		return
	}

	plaintext, tokenHash, err := crypto.NewURLToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	expiry := time.Now().Add(inviteExpiry)
	if _, err := h.store.CreateInvitation(r.Context(), claims.TenantID, req.Email, req.Role, tokenHash, expiry, nil); err != nil {
		slog.Error("creating invitation", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create invitation")
		return
	}
	inviteURL := h.tenantWebURL + "/accept-invite?token=" + plaintext
	if err := h.mailer.SendInvite(req.Email, inviteURL, tenant.Name); err != nil {
		slog.Warn("sending invitation email", "error", err)
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status": "created_but_email_failed",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "invited"})
}

// DELETE /api/v1/tenant-auth/members/{user_id} (authenticated, admin-only)
func (h *TenantAuthHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetTenantClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if claims.Role != model.MembershipRoleAdmin {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}
	userID := r.PathValue("user_id")
	if userID == claims.Sub {
		writeError(w, http.StatusBadRequest, "cannot remove yourself")
		return
	}
	if err := h.store.DeleteMembership(r.Context(), userID, claims.TenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "membership not found")
			return
		}
		slog.Error("removing member", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
