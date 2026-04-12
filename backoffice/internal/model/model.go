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
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
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

type DashboardStats struct {
	DataCenters []DataCenterStats `json:"data_centers"`
}

type DataCenterStats struct {
	DataCenter DataCenter      `json:"data_center"`
	Stats      *DCStatsResponse `json:"stats,omitempty"`
	Error      string          `json:"error,omitempty"`
}
