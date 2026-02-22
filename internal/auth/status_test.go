package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetAuthStatus_NotAuthenticated(t *testing.T) {
	// Clear any env vars that could affect the test.
	for _, env := range []string{
		"CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY",
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY",
	} {
		t.Setenv(env, "")
	}

	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}

	status := GetAuthStatus(store)

	if status.LoggedIn {
		t.Error("expected LoggedIn to be false")
	}
	if status.AuthMethod != AuthMethodNone {
		t.Errorf("expected AuthMethod=%q, got %q", AuthMethodNone, status.AuthMethod)
	}
	if status.APIProvider != APIProviderFirstParty {
		t.Errorf("expected APIProvider=%q, got %q", APIProviderFirstParty, status.APIProvider)
	}
}

func TestGetAuthStatus_OAuthToken(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "test-token-123")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "")

	status := GetAuthStatus(nil)

	if !status.LoggedIn {
		t.Error("expected LoggedIn to be true")
	}
	if status.AuthMethod != AuthMethodOAuthToken {
		t.Errorf("expected AuthMethod=%q, got %q", AuthMethodOAuthToken, status.AuthMethod)
	}
	if status.APIKeySource != "CLAUDE_CODE_OAUTH_TOKEN" {
		t.Errorf("expected APIKeySource=%q, got %q", "CLAUDE_CODE_OAUTH_TOKEN", status.APIKeySource)
	}
}

func TestGetAuthStatus_APIKey(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "")

	status := GetAuthStatus(nil)

	if !status.LoggedIn {
		t.Error("expected LoggedIn to be true")
	}
	if status.AuthMethod != AuthMethodAPIKey {
		t.Errorf("expected AuthMethod=%q, got %q", AuthMethodAPIKey, status.AuthMethod)
	}
	if status.APIKeySource != "ANTHROPIC_API_KEY" {
		t.Errorf("expected APIKeySource=%q, got %q", "ANTHROPIC_API_KEY", status.APIKeySource)
	}
}

func TestGetAuthStatus_ThirdParty_Bedrock(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "")

	status := GetAuthStatus(nil)

	if !status.LoggedIn {
		t.Error("expected LoggedIn to be true")
	}
	if status.AuthMethod != AuthMethodThirdParty {
		t.Errorf("expected AuthMethod=%q, got %q", AuthMethodThirdParty, status.AuthMethod)
	}
	if status.APIProvider != APIProviderBedrock {
		t.Errorf("expected APIProvider=%q, got %q", APIProviderBedrock, status.APIProvider)
	}
}

func TestGetAuthStatus_ThirdParty_Vertex(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "1")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "")

	status := GetAuthStatus(nil)

	if !status.LoggedIn {
		t.Error("expected LoggedIn to be true")
	}
	if status.AuthMethod != AuthMethodThirdParty {
		t.Errorf("expected AuthMethod=%q, got %q", AuthMethodThirdParty, status.AuthMethod)
	}
	if status.APIProvider != APIProviderVertex {
		t.Errorf("expected APIProvider=%q, got %q", APIProviderVertex, status.APIProvider)
	}
}

func TestGetAuthStatus_ThirdParty_Foundry(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "1")

	status := GetAuthStatus(nil)

	if !status.LoggedIn {
		t.Error("expected LoggedIn to be true")
	}
	if status.AuthMethod != AuthMethodThirdParty {
		t.Errorf("expected AuthMethod=%q, got %q", AuthMethodThirdParty, status.AuthMethod)
	}
	if status.APIProvider != APIProviderFoundry {
		t.Errorf("expected APIProvider=%q, got %q", APIProviderFoundry, status.APIProvider)
	}
}

func TestGetAuthStatus_ClaudeAI(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "")

	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}

	// Write test credentials.
	creds := credentialsFile{
		ClaudeAiOauth: &OAuthTokens{
			AccessToken:      "test-access-token",
			RefreshToken:     "test-refresh-token",
			ExpiresAt:        9999999999999,
			SubscriptionType: "pro",
		},
		OAuthAccount: &OAuthAccount{
			EmailAddress:     "user@example.com",
			OrganizationUUID: "org-uuid-123",
			OrganizationName: "Test Org",
		},
	}
	data, _ := json.Marshal(creds)
	os.WriteFile(store.path, data, 0600)

	status := GetAuthStatus(store)

	if !status.LoggedIn {
		t.Error("expected LoggedIn to be true")
	}
	if status.AuthMethod != AuthMethodClaudeAI {
		t.Errorf("expected AuthMethod=%q, got %q", AuthMethodClaudeAI, status.AuthMethod)
	}
	if status.Email == nil || *status.Email != "user@example.com" {
		t.Errorf("expected Email=%q, got %v", "user@example.com", status.Email)
	}
	if status.OrgID == nil || *status.OrgID != "org-uuid-123" {
		t.Errorf("expected OrgID=%q, got %v", "org-uuid-123", status.OrgID)
	}
	if status.OrgName == nil || *status.OrgName != "Test Org" {
		t.Errorf("expected OrgName=%q, got %v", "Test Org", status.OrgName)
	}
	if status.SubscriptionType == nil || *status.SubscriptionType != "Claude Pro" {
		t.Errorf("expected SubscriptionType=%q, got %v", "Claude Pro", status.SubscriptionType)
	}
}

func TestGetAuthStatus_Priority_ThirdPartyOverEnvVars(t *testing.T) {
	// Third-party should take priority over CLAUDE_CODE_OAUTH_TOKEN.
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "some-token")
	t.Setenv("ANTHROPIC_API_KEY", "some-key")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "")

	status := GetAuthStatus(nil)

	if status.AuthMethod != AuthMethodThirdParty {
		t.Errorf("expected AuthMethod=%q, got %q", AuthMethodThirdParty, status.AuthMethod)
	}
}

func TestGetAuthStatus_Priority_OAuthTokenOverAPIKey(t *testing.T) {
	// CLAUDE_CODE_OAUTH_TOKEN should take priority over ANTHROPIC_API_KEY.
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-token")
	t.Setenv("ANTHROPIC_API_KEY", "api-key")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "")

	status := GetAuthStatus(nil)

	if status.AuthMethod != AuthMethodOAuthToken {
		t.Errorf("expected AuthMethod=%q, got %q", AuthMethodOAuthToken, status.AuthMethod)
	}
}

func TestFormatStatusJSON(t *testing.T) {
	email := "user@example.com"
	orgName := "Test Org"
	subType := "Claude Pro"
	status := &AuthStatus{
		LoggedIn:         true,
		AuthMethod:       AuthMethodClaudeAI,
		APIProvider:      APIProviderFirstParty,
		Email:            &email,
		OrgName:          &orgName,
		SubscriptionType: &subType,
	}

	output, err := FormatStatusJSON(status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if parsed["loggedIn"] != true {
		t.Error("expected loggedIn=true in JSON")
	}
	if parsed["authMethod"] != "claude.ai" {
		t.Errorf("expected authMethod=%q, got %v", "claude.ai", parsed["authMethod"])
	}
	if parsed["email"] != "user@example.com" {
		t.Errorf("expected email=%q, got %v", "user@example.com", parsed["email"])
	}
}

func TestFormatStatusText_LoggedIn(t *testing.T) {
	email := "user@example.com"
	orgName := "Test Org"
	subType := "Claude Pro"
	status := &AuthStatus{
		LoggedIn:         true,
		AuthMethod:       AuthMethodClaudeAI,
		APIProvider:      APIProviderFirstParty,
		Email:            &email,
		OrgName:          &orgName,
		SubscriptionType: &subType,
	}

	output := FormatStatusText(status)

	if !strings.Contains(output, "Claude Pro Account") {
		t.Errorf("expected output to contain 'Claude Pro Account', got: %s", output)
	}
	if !strings.Contains(output, "Organization: Test Org") {
		t.Errorf("expected output to contain 'Organization: Test Org', got: %s", output)
	}
	if !strings.Contains(output, "Email: user@example.com") {
		t.Errorf("expected output to contain 'Email: user@example.com', got: %s", output)
	}
}

func TestFormatStatusText_NotLoggedIn(t *testing.T) {
	status := &AuthStatus{
		LoggedIn:   false,
		AuthMethod: AuthMethodNone,
	}

	output := FormatStatusText(status)

	if !strings.Contains(output, "Not logged in") {
		t.Errorf("expected output to contain 'Not logged in', got: %s", output)
	}
}

func TestFormatStatusText_APIKey(t *testing.T) {
	status := &AuthStatus{
		LoggedIn:    true,
		AuthMethod:  AuthMethodAPIKey,
		APIProvider: APIProviderFirstParty,
		APIKeySource: "ANTHROPIC_API_KEY",
	}

	output := FormatStatusText(status)

	if !strings.Contains(output, "Login method: API Key") {
		t.Errorf("expected output to contain 'Login method: API Key', got: %s", output)
	}
	if !strings.Contains(output, "API key source: ANTHROPIC_API_KEY") {
		t.Errorf("expected output to contain 'API key source: ANTHROPIC_API_KEY', got: %s", output)
	}
}

func TestFormatStatusText_ThirdPartyProvider(t *testing.T) {
	status := &AuthStatus{
		LoggedIn:    true,
		AuthMethod:  AuthMethodThirdParty,
		APIProvider: APIProviderBedrock,
	}

	output := FormatStatusText(status)

	if !strings.Contains(output, "Third-Party Provider") {
		t.Errorf("expected output to contain 'Third-Party Provider', got: %s", output)
	}
	if !strings.Contains(output, "API provider: AWS Bedrock") {
		t.Errorf("expected output to contain 'API provider: AWS Bedrock', got: %s", output)
	}
}

func TestSubscriptionDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"enterprise", "Claude Enterprise"},
		{"team", "Claude Team"},
		{"max", "Claude Max"},
		{"pro", "Claude Pro"},
		{"Pro", "Claude Pro"},
		{"unknown", "Claude API"},
		{"", "Claude API"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := subscriptionDisplayName(tt.input)
			if got != tt.expected {
				t.Errorf("subscriptionDisplayName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatStatusJSON_NotLoggedIn(t *testing.T) {
	status := &AuthStatus{
		LoggedIn:    false,
		AuthMethod:  AuthMethodNone,
		APIProvider: APIProviderFirstParty,
	}

	output, err := FormatStatusJSON(status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if parsed["loggedIn"] != false {
		t.Error("expected loggedIn=false in JSON")
	}
	if parsed["authMethod"] != "none" {
		t.Errorf("expected authMethod=%q, got %v", "none", parsed["authMethod"])
	}
	// email, orgId, orgName, subscriptionType should be null.
	if parsed["email"] != nil {
		t.Errorf("expected email=nil, got %v", parsed["email"])
	}
}
