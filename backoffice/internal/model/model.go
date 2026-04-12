package model

import (
	"encoding/json"
	"time"
)

// Status constants for tenants.
const (
	TenantStatusActive   = "active"
	TenantStatusSuspended = "suspended"
	TenantStatusInactive = "inactive"
)

// Status constants for data centers.
const (
	DCStatusActive   = "active"
	DCStatusInactive = "inactive"
)

// Environment constants for data centers.
const (
	DCEnvStage = "stage"
	DCEnvProd  = "prod"
)

// Provisioning status constants.
const (
	ProvisioningPending     = "pending"
	ProvisioningProvisioned = "provisioned"
	ProvisioningFailed      = "failed"
)

// Admin user roles.
const (
	RoleViewer     = "viewer"
	RoleAdmin      = "admin"
	RoleSuperAdmin = "super_admin"
)

// Tenant user (membership) roles.
const (
	MembershipRoleAdmin  = "admin"
	MembershipRoleMember = "member"
)

// MaxMembershipsPerUser caps how many tenants a single user can belong to.
// Prevents abuse/runaway invitation of one account across thousands of orgs.
const MaxMembershipsPerUser = 20

// User is an end-user that authenticates into the tenant frontend.
// Distinct from AdminUser (backoffice admin).
type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	PasswordHash    string     `json:"-"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Membership struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TenantID  string    `json:"tenant_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// TenantMember is a joined view of memberships + users for the Team page.
type TenantMember struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// MembershipSummary is what we return to the tenant frontend so it knows
// which tenants a user can switch between and where each one lives.
type MembershipSummary struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	DCID       string `json:"dc_id"`
	DCAPIURL   string `json:"dc_api_url"`
	Role       string `json:"role"`
}

type Invitation struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	Email          string     `json:"email"`
	Role           string     `json:"role"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	InvitedByAdmin *string    `json:"invited_by_admin,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type DataCenter struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Region            string     `json:"region"`
	Environment       string     `json:"environment"` // "stage" or "prod"
	APIURL            string     `json:"api_url"`
	APIKeyEncrypted   []byte     `json:"-"`
	Status            string     `json:"status"`
	LastHealthCheck   *time.Time `json:"last_health_check,omitempty"`
	LastHealthStatus  string     `json:"last_health_status"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Tenant struct {
	ID                 string          `json:"id"`
	DCTenantID         *string         `json:"dc_tenant_id,omitempty"`
	DataCenterID       string          `json:"data_center_id"`
	Name               string          `json:"name"`
	Status             string          `json:"status"`
	Config             json.RawMessage `json:"config"`
	ProvisioningStatus string          `json:"provisioning_status"`
	ClerkOrgID         *string         `json:"clerk_org_id,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	// InviteResults is only populated on the Create response. Not persisted.
	InviteResults []InviteResult `json:"invite_results,omitempty"`
}

type AdminUser struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// Request types

type CreateDataCenterRequest struct {
	Name        string `json:"name"`
	Region      string `json:"region"`
	Environment string `json:"environment"` // "stage" or "prod"
	APIURL      string `json:"api_url"`
	APIKey      string `json:"api_key"`
}

type UpdateDataCenterRequest struct {
	Name        *string `json:"name,omitempty"`
	Region      *string `json:"region,omitempty"`
	Environment *string `json:"environment,omitempty"`
	APIURL      *string `json:"api_url,omitempty"`
	APIKey      *string `json:"api_key,omitempty"`
	Status      *string `json:"status,omitempty"`
}

type CreateTenantRequest struct {
	DataCenterID string          `json:"data_center_id"`
	Name         string          `json:"name"`
	Config       json.RawMessage `json:"config,omitempty"`
	Invites      []TenantInvite  `json:"invites,omitempty"`
}

// Invite role constants (simple UI-facing values; mapped to Clerk roles in handler).
const (
	InviteRoleAdmin = "admin"
	InviteRoleBasic = "basic"
)

type TenantInvite struct {
	Email string `json:"email"`
	Role  string `json:"role"` // "admin" or "basic"
}

// InviteResult is returned in the Create response (not persisted).
type InviteResult struct {
	Email  string `json:"email"`
	Role   string `json:"role"`
	Status string `json:"status"` // "invited" or "failed"
	Error  string `json:"error,omitempty"`
}

type UpdateTenantRequest struct {
	Name   *string          `json:"name,omitempty"`
	Config json.RawMessage  `json:"config,omitempty"`
}

type UpdateTenantStatusRequest struct {
	Status string `json:"status"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string    `json:"token"`
	Admin AdminUser `json:"admin"`
}

// DC Client response types

type DCTenantResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DCStatsResponse struct {
	TenantCount int `json:"tenant_count"`
	AgentCount  int `json:"agent_count"`
	ScanCount   int `json:"scan_count"`
}

type DCAgentResponse struct {
	ID       string  `json:"id"`
	TenantID string  `json:"tenant_id"`
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	Version  string  `json:"version"`
}

// DataCenterListItem is what the list endpoint returns — base DC plus a
// computed tenant_count from the backoffice DB.
type DataCenterListItem struct {
	DataCenter
	TenantCount int `json:"tenant_count"`
}

type DashboardStats struct {
	TotalDataCenters int                  `json:"total_data_centers"`
	TotalTenants     int                  `json:"total_tenants"`
	ActiveTenants    int                  `json:"active_tenants"`
	SuspendedTenants int                  `json:"suspended_tenants"`
	DataCenters      []DataCenterWithStats `json:"data_centers"`
}

type DataCenterWithStats struct {
	DataCenter
	TenantCount int    `json:"tenant_count"`
	AgentCount  int    `json:"agent_count"`
	ScanCount   int    `json:"scan_count"`
	StatsError  string `json:"stats_error,omitempty"`
}
