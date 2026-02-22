package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OAuthAccount holds account metadata stored alongside OAuth credentials.
// Matches the JS version's oauthAccount structure.
type OAuthAccount struct {
	AccountUUID          string `json:"accountUuid,omitempty"`
	EmailAddress         string `json:"emailAddress,omitempty"`
	OrganizationUUID     string `json:"organizationUuid,omitempty"`
	DisplayName          string `json:"displayName,omitempty"`
	HasExtraUsageEnabled bool   `json:"hasExtraUsageEnabled,omitempty"`
	BillingType          string `json:"billingType,omitempty"`
	OrganizationRole     string `json:"organizationRole,omitempty"`
	WorkspaceRole        string `json:"workspaceRole,omitempty"`
	OrganizationName     string `json:"organizationName,omitempty"`
}

// ProfileResponse is the JSON structure returned by GET /api/oauth/profile.
type ProfileResponse struct {
	Account struct {
		UUID        string `json:"uuid"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		CreatedAt   string `json:"created_at"`
	} `json:"account"`
	Organization struct {
		UUID                    string `json:"uuid"`
		OrganizationType        string `json:"organization_type"`
		RateLimitTier           string `json:"rate_limit_tier"`
		HasExtraUsageEnabled    bool   `json:"has_extra_usage_enabled"`
		BillingType             string `json:"billing_type"`
		SubscriptionCreatedAt   string `json:"subscription_created_at"`
	} `json:"organization"`
}

// ProfileInfo holds processed profile data used to populate token and account fields.
type ProfileInfo struct {
	SubscriptionType      string
	RateLimitTier         string
	HasExtraUsageEnabled  bool
	BillingType           string
	DisplayName           string
	AccountCreatedAt      string
	SubscriptionCreatedAt string
}

// RolesResponse is the JSON structure returned by GET /api/oauth/claude_cli/roles.
type RolesResponse struct {
	OrganizationRole string `json:"organization_role"`
	WorkspaceRole    string `json:"workspace_role"`
	OrganizationName string `json:"organization_name"`
}

// APIKeyResponse is the JSON structure returned by POST /api/oauth/claude_cli/create_api_key.
type APIKeyResponse struct {
	RawKey string `json:"raw_key"`
}

// FetchProfileInfo calls GET {baseAPIURL}/api/oauth/profile and returns processed profile info.
func FetchProfileInfo(ctx context.Context, baseAPIURL, accessToken string) (*ProfileInfo, error) {
	url := baseAPIURL + "/api/oauth/profile"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("profile request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading profile response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("profile fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var profile ProfileResponse
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, fmt.Errorf("parsing profile response: %w", err)
	}

	return processProfile(&profile), nil
}

func processProfile(p *ProfileResponse) *ProfileInfo {
	info := &ProfileInfo{
		RateLimitTier:         p.Organization.RateLimitTier,
		HasExtraUsageEnabled:  p.Organization.HasExtraUsageEnabled,
		BillingType:           p.Organization.BillingType,
		DisplayName:           p.Account.DisplayName,
		AccountCreatedAt:      p.Account.CreatedAt,
		SubscriptionCreatedAt: p.Organization.SubscriptionCreatedAt,
	}

	switch p.Organization.OrganizationType {
	case "claude_max":
		info.SubscriptionType = "max"
	case "claude_pro":
		info.SubscriptionType = "pro"
	case "claude_enterprise":
		info.SubscriptionType = "enterprise"
	case "claude_team":
		info.SubscriptionType = "team"
	}

	return info
}

// FetchRoles calls GET {rolesURL} and returns role information.
func FetchRoles(ctx context.Context, rolesURL, accessToken string) (*RolesResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rolesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating roles request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("roles request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading roles response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("roles fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var roles RolesResponse
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, fmt.Errorf("parsing roles response: %w", err)
	}

	return &roles, nil
}

// CreateAPIKey calls POST {apiKeyURL} to create a new API key.
func CreateAPIKey(ctx context.Context, apiKeyURL, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", apiKeyURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating API key request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API key request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading API key response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API key creation failed (%d): %s", resp.StatusCode, string(body))
	}

	var apiKeyResp APIKeyResponse
	if err := json.Unmarshal(body, &apiKeyResp); err != nil {
		return "", fmt.Errorf("parsing API key response: %w", err)
	}

	return apiKeyResp.RawKey, nil
}
