// Package clerkclient is a minimal HTTP client for Clerk's Backend API.
// Only the organization endpoints we need are implemented.
package clerkclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const clerkAPIBase = "https://api.clerk.com/v1"

// Client calls the Clerk Backend API using a secret key.
// If SecretKey is empty, all methods return ErrDisabled.
type Client struct {
	SecretKey string
	http      *http.Client
}

// New constructs a new Clerk client. An empty secretKey disables all calls
// (methods return ErrDisabled) so local dev without Clerk still works.
func New(secretKey string) *Client {
	return &Client{
		SecretKey: secretKey,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

// ErrDisabled is returned when the client has no secret key configured.
var ErrDisabled = fmt.Errorf("clerk client disabled (no secret key)")

// Organization is the subset of Clerk's organization response we care about.
type Organization struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Slug     string                 `json:"slug,omitempty"`
	Metadata map[string]interface{} `json:"public_metadata,omitempty"`
}

// CreateOrganizationRequest is the payload for creating a Clerk org.
type CreateOrganizationRequest struct {
	Name           string                 `json:"name"`
	Slug           string                 `json:"slug,omitempty"`
	CreatedBy      string                 `json:"created_by,omitempty"` // Clerk user_id who becomes the admin
	PublicMetadata map[string]interface{} `json:"public_metadata,omitempty"`
}

// CreateOrganization creates a new Clerk organization.
// CreatedBy is required by Clerk — the user ID that becomes the initial admin.
func (c *Client) CreateOrganization(req CreateOrganizationRequest) (*Organization, error) {
	if c.SecretKey == "" {
		return nil, ErrDisabled
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling create org request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, clerkAPIBase+"/organizations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.SecretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("clerk create org: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("clerk create org returned %d: %s", resp.StatusCode, string(b))
	}

	var org Organization
	if err := json.NewDecoder(resp.Body).Decode(&org); err != nil {
		return nil, fmt.Errorf("decoding create org response: %w", err)
	}
	return &org, nil
}

// DeleteOrganization removes a Clerk organization.
func (c *Client) DeleteOrganization(orgID string) error {
	if c.SecretKey == "" {
		return ErrDisabled
	}

	httpReq, err := http.NewRequest(http.MethodDelete, clerkAPIBase+"/organizations/"+orgID, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.SecretKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("clerk delete org: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("clerk delete org returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// Membership counts from the org memberships endpoint.
type membershipsResponse struct {
	Data       []membership `json:"data"`
	TotalCount int          `json:"total_count"`
}

type membership struct {
	Role string `json:"role"`
}

// OrganizationStats returns a count of members and admins in an org.
type OrganizationStats struct {
	MemberCount int `json:"member_count"`
	AdminCount  int `json:"admin_count"`
}

// GetOrganizationStats returns member counts for the org.
func (c *Client) GetOrganizationStats(orgID string) (*OrganizationStats, error) {
	if c.SecretKey == "" {
		return nil, ErrDisabled
	}

	httpReq, err := http.NewRequest(http.MethodGet, clerkAPIBase+"/organizations/"+orgID+"/memberships?limit=500", nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.SecretKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("clerk list memberships: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("clerk list memberships returned %d: %s", resp.StatusCode, string(b))
	}

	var r membershipsResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decoding memberships response: %w", err)
	}

	stats := &OrganizationStats{MemberCount: r.TotalCount}
	for _, m := range r.Data {
		if m.Role == "admin" || m.Role == "org:admin" {
			stats.AdminCount++
		}
	}
	return stats, nil
}
