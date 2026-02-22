package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ===========================================================================
// Issue 6: Profile fetch after token exchange
// ===========================================================================

func TestFetchProfileInfo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request.
		if r.URL.Path != "/api/oauth/profile" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("unexpected Authorization: %s", auth)
		}

		json.NewEncoder(w).Encode(ProfileResponse{
			Account: struct {
				UUID        string `json:"uuid"`
				Email       string `json:"email"`
				DisplayName string `json:"display_name"`
				CreatedAt   string `json:"created_at"`
			}{
				UUID:        "acct-uuid",
				Email:       "user@example.com",
				DisplayName: "Test User",
				CreatedAt:   "2024-01-01",
			},
			Organization: struct {
				UUID                  string `json:"uuid"`
				OrganizationType      string `json:"organization_type"`
				RateLimitTier         string `json:"rate_limit_tier"`
				HasExtraUsageEnabled  bool   `json:"has_extra_usage_enabled"`
				BillingType           string `json:"billing_type"`
				SubscriptionCreatedAt string `json:"subscription_created_at"`
			}{
				UUID:                  "org-uuid",
				OrganizationType:      "claude_pro",
				RateLimitTier:         "tier_2",
				HasExtraUsageEnabled:  true,
				BillingType:           "stripe",
				SubscriptionCreatedAt: "2024-02-01",
			},
		})
	}))
	defer server.Close()

	info, err := FetchProfileInfo(context.Background(), server.URL, "test-token")
	if err != nil {
		t.Fatalf("FetchProfileInfo error: %v", err)
	}
	if info.SubscriptionType != "pro" {
		t.Errorf("SubscriptionType: got %q, want %q", info.SubscriptionType, "pro")
	}
	if info.RateLimitTier != "tier_2" {
		t.Errorf("RateLimitTier: got %q", info.RateLimitTier)
	}
	if !info.HasExtraUsageEnabled {
		t.Error("HasExtraUsageEnabled should be true")
	}
	if info.BillingType != "stripe" {
		t.Errorf("BillingType: got %q", info.BillingType)
	}
	if info.DisplayName != "Test User" {
		t.Errorf("DisplayName: got %q", info.DisplayName)
	}
	if info.AccountCreatedAt != "2024-01-01" {
		t.Errorf("AccountCreatedAt: got %q", info.AccountCreatedAt)
	}
	if info.SubscriptionCreatedAt != "2024-02-01" {
		t.Errorf("SubscriptionCreatedAt: got %q", info.SubscriptionCreatedAt)
	}
}

func TestFetchProfileInfo_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	_, err := FetchProfileInfo(context.Background(), server.URL, "tok")
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestProcessProfile_OrganizationTypeMapping(t *testing.T) {
	tests := []struct {
		orgType string
		want    string
	}{
		{"claude_pro", "pro"},
		{"claude_max", "max"},
		{"claude_enterprise", "enterprise"},
		{"claude_team", "team"},
		{"claude_free", ""},
		{"unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.orgType, func(t *testing.T) {
			p := &ProfileResponse{}
			p.Organization.OrganizationType = tt.orgType
			info := processProfile(p)
			if info.SubscriptionType != tt.want {
				t.Errorf("orgType %q: got SubscriptionType %q, want %q",
					tt.orgType, info.SubscriptionType, tt.want)
			}
		})
	}
}

// ===========================================================================
// Issue 7: Roles fetch
// ===========================================================================

func TestFetchRoles_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer roles-token" {
			t.Errorf("unexpected Authorization: %s", auth)
		}

		json.NewEncoder(w).Encode(RolesResponse{
			OrganizationRole: "admin",
			WorkspaceRole:    "developer",
			OrganizationName: "Acme Corp",
		})
	}))
	defer server.Close()

	roles, err := FetchRoles(context.Background(), server.URL, "roles-token")
	if err != nil {
		t.Fatalf("FetchRoles error: %v", err)
	}
	if roles.OrganizationRole != "admin" {
		t.Errorf("OrganizationRole: got %q", roles.OrganizationRole)
	}
	if roles.WorkspaceRole != "developer" {
		t.Errorf("WorkspaceRole: got %q", roles.WorkspaceRole)
	}
	if roles.OrganizationName != "Acme Corp" {
		t.Errorf("OrganizationName: got %q", roles.OrganizationName)
	}
}

func TestFetchRoles_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	_, err := FetchRoles(context.Background(), server.URL, "tok")
	if err == nil {
		t.Fatal("expected error on 403")
	}
}

// ===========================================================================
// Issue 8: API key creation
// ===========================================================================

func TestCreateAPIKey_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer create-key-token" {
			t.Errorf("unexpected Authorization: %s", auth)
		}

		json.NewEncoder(w).Encode(APIKeyResponse{
			RawKey: "sk-ant-api-key-123",
		})
	}))
	defer server.Close()

	key, err := CreateAPIKey(context.Background(), server.URL, "create-key-token")
	if err != nil {
		t.Fatalf("CreateAPIKey error: %v", err)
	}
	if key != "sk-ant-api-key-123" {
		t.Errorf("expected sk-ant-api-key-123, got %q", key)
	}
}

func TestCreateAPIKey_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	_, err := CreateAPIKey(context.Background(), server.URL, "tok")
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestCreateAPIKey_EmptyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIKeyResponse{RawKey: ""})
	}))
	defer server.Close()

	key, err := CreateAPIKey(context.Background(), server.URL, "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "" {
		t.Errorf("expected empty key, got %q", key)
	}
}
