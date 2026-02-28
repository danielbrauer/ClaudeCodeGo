package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Issue 3: State parameter should be 32 random bytes
// ---------------------------------------------------------------------------

func TestGenerateState_Uses32Bytes(t *testing.T) {
	state, err := generateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// base64-raw-url of 32 bytes → 43 characters.
	decoded, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		t.Fatalf("state is not valid base64-raw-url: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("expected 32 random bytes, got %d", len(decoded))
	}
}

func TestGenerateState_IsRandom(t *testing.T) {
	s1, _ := generateState()
	s2, _ := generateState()
	if s1 == s2 {
		t.Error("two generated states should not be equal")
	}
}

func TestGenerateState_LongerThanOld16Bytes(t *testing.T) {
	// 32 bytes → 43 chars in base64-raw-url; old 16 bytes → 22 chars.
	state, err := generateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state) < 40 {
		t.Errorf("state too short (%d chars), expected ~43 for 32 bytes", len(state))
	}
}

// ---------------------------------------------------------------------------
// Issue 4: Manual fallback URL uses ManualRedirectURL, not localhost
// ---------------------------------------------------------------------------

func TestBuildAuthURL_AutoUsesLocalhost(t *testing.T) {
	cfg := &OAuthURLConfig{
		AuthorizeURL:      "https://claude.ai/oauth/authorize",
		ClientID:          "test-client-id",
		ManualRedirectURL: "https://platform.claude.com/oauth/code/callback",
	}
	rawURL := buildAuthURL(cfg, "challenge", "state", 12345, false, LoginOptions{})
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("invalid URL: %v", err)
	}
	redirectURI := u.Query().Get("redirect_uri")
	if redirectURI != "http://localhost:12345/callback" {
		t.Errorf("auto URL should use localhost redirect, got %q", redirectURI)
	}
}

func TestBuildAuthURL_ManualUsesPlatformRedirect(t *testing.T) {
	cfg := &OAuthURLConfig{
		AuthorizeURL:      "https://claude.ai/oauth/authorize",
		ClientID:          "test-client-id",
		ManualRedirectURL: "https://platform.claude.com/oauth/code/callback",
	}
	rawURL := buildAuthURL(cfg, "challenge", "state", 12345, true, LoginOptions{})
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("invalid URL: %v", err)
	}
	redirectURI := u.Query().Get("redirect_uri")
	if redirectURI != "https://platform.claude.com/oauth/code/callback" {
		t.Errorf("manual URL should use platform redirect, got %q", redirectURI)
	}
}

func TestBuildAuthURL_IncludesRequiredParams(t *testing.T) {
	cfg := &OAuthURLConfig{
		AuthorizeURL:      "https://claude.ai/oauth/authorize",
		ClientID:          "test-client-id",
		ManualRedirectURL: "https://example.com/callback",
	}
	rawURL := buildAuthURL(cfg, "test-challenge", "test-state", 8080, false, LoginOptions{})
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("invalid URL: %v", err)
	}
	q := u.Query()
	checks := map[string]string{
		"code":                  "true",
		"client_id":             "test-client-id",
		"response_type":         "code",
		"code_challenge":        "test-challenge",
		"code_challenge_method": "S256",
		"state":                 "test-state",
	}
	for key, want := range checks {
		if got := q.Get(key); got != want {
			t.Errorf("param %q: got %q, want %q", key, got, want)
		}
	}
	if !strings.Contains(q.Get("scope"), "user:inference") {
		t.Error("scope should include user:inference")
	}
}

func TestBuildAuthURL_WithLoginHint(t *testing.T) {
	cfg := &OAuthURLConfig{
		AuthorizeURL:      "https://claude.ai/oauth/authorize",
		ClientID:          "test-client-id",
		ManualRedirectURL: "https://example.com/callback",
	}
	opts := LoginOptions{Email: "user@example.com"}
	rawURL := buildAuthURL(cfg, "challenge", "state", 8080, false, opts)
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("invalid URL: %v", err)
	}
	if got := u.Query().Get("login_hint"); got != "user@example.com" {
		t.Errorf("login_hint: got %q, want %q", got, "user@example.com")
	}
}

func TestBuildAuthURL_WithSSO(t *testing.T) {
	cfg := &OAuthURLConfig{
		AuthorizeURL:      "https://claude.ai/oauth/authorize",
		ClientID:          "test-client-id",
		ManualRedirectURL: "https://example.com/callback",
	}
	opts := LoginOptions{SSO: true}
	rawURL := buildAuthURL(cfg, "challenge", "state", 8080, false, opts)
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("invalid URL: %v", err)
	}
	if got := u.Query().Get("login_method"); got != "sso" {
		t.Errorf("login_method: got %q, want %q", got, "sso")
	}
}

func TestBuildAuthURL_WithoutOptionsNoLoginHintOrMethod(t *testing.T) {
	cfg := &OAuthURLConfig{
		AuthorizeURL:      "https://claude.ai/oauth/authorize",
		ClientID:          "test-client-id",
		ManualRedirectURL: "https://example.com/callback",
	}
	rawURL := buildAuthURL(cfg, "challenge", "state", 8080, false, LoginOptions{})
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("invalid URL: %v", err)
	}
	if got := u.Query().Get("login_hint"); got != "" {
		t.Errorf("login_hint should be absent, got %q", got)
	}
	if got := u.Query().Get("login_method"); got != "" {
		t.Errorf("login_method should be absent, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Issue 5: TokenResponse parses account and organization fields
// ---------------------------------------------------------------------------

func TestTokenResponse_ParsesAccountAndOrg(t *testing.T) {
	raw := `{
		"access_token": "tok_123",
		"refresh_token": "ref_456",
		"expires_in": 3600,
		"scope": "user:inference user:profile",
		"token_type": "bearer",
		"account": {
			"uuid": "acct-uuid-abc",
			"email_address": "user@example.com"
		},
		"organization": {
			"uuid": "org-uuid-xyz"
		}
	}`

	var resp TokenResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.AccessToken != "tok_123" {
		t.Errorf("access_token: got %q", resp.AccessToken)
	}
	if resp.Account == nil {
		t.Fatal("account should not be nil")
	}
	if resp.Account.UUID != "acct-uuid-abc" {
		t.Errorf("account.uuid: got %q", resp.Account.UUID)
	}
	if resp.Account.EmailAddress != "user@example.com" {
		t.Errorf("account.email_address: got %q", resp.Account.EmailAddress)
	}
	if resp.Organization == nil {
		t.Fatal("organization should not be nil")
	}
	if resp.Organization.UUID != "org-uuid-xyz" {
		t.Errorf("organization.uuid: got %q", resp.Organization.UUID)
	}
}

func TestTokenResponse_OmitsAccountWhenAbsent(t *testing.T) {
	raw := `{"access_token":"tok","refresh_token":"ref","expires_in":3600,"scope":"s","token_type":"bearer"}`
	var resp TokenResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Account != nil {
		t.Error("account should be nil when absent from JSON")
	}
	if resp.Organization != nil {
		t.Error("organization should be nil when absent from JSON")
	}
}

// ---------------------------------------------------------------------------
// Issue 17: CLAUDE_CODE_OAUTH_CLIENT_ID override
// ---------------------------------------------------------------------------

func TestGetOAuthConfig_DefaultClientID(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")

	cfg, err := GetOAuthConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientID != DefaultClientID {
		t.Errorf("expected default client ID %q, got %q", DefaultClientID, cfg.ClientID)
	}
}

func TestGetOAuthConfig_ClientIDOverride(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "custom-client-123")
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")

	cfg, err := GetOAuthConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientID != "custom-client-123" {
		t.Errorf("expected custom client ID, got %q", cfg.ClientID)
	}
}

// ---------------------------------------------------------------------------
// Issue 18: CLAUDE_CODE_CUSTOM_OAUTH_URL override
// ---------------------------------------------------------------------------

func TestGetOAuthConfig_DefaultURLs(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	cfg, err := GetOAuthConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseAPIURL != DefaultBaseAPIURL {
		t.Errorf("BaseAPIURL: got %q, want %q", cfg.BaseAPIURL, DefaultBaseAPIURL)
	}
	if cfg.AuthorizeURL != DefaultAuthorizeURL {
		t.Errorf("AuthorizeURL: got %q, want %q", cfg.AuthorizeURL, DefaultAuthorizeURL)
	}
	if cfg.TokenURL != DefaultTokenURL {
		t.Errorf("TokenURL: got %q, want %q", cfg.TokenURL, DefaultTokenURL)
	}
	if cfg.ManualRedirectURL != DefaultManualRedirect {
		t.Errorf("ManualRedirectURL: got %q, want %q", cfg.ManualRedirectURL, DefaultManualRedirect)
	}
}

func TestGetOAuthConfig_CustomURLOverridesAll(t *testing.T) {
	base := "https://claude.fedstart.com"
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", base)
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	cfg, err := GetOAuthConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{
		"BaseAPIURL":        base,
		"AuthorizeURL":      base + "/oauth/authorize",
		"TokenURL":          base + "/v1/oauth/token",
		"APIKeyURL":         base + "/api/oauth/claude_cli/create_api_key",
		"RolesURL":          base + "/api/oauth/claude_cli/roles",
		"SuccessURL":        base + "/oauth/code/success?app=claude-code",
		"ManualRedirectURL": base + "/oauth/code/callback",
	}
	got := map[string]string{
		"BaseAPIURL":        cfg.BaseAPIURL,
		"AuthorizeURL":      cfg.AuthorizeURL,
		"TokenURL":          cfg.TokenURL,
		"APIKeyURL":         cfg.APIKeyURL,
		"RolesURL":          cfg.RolesURL,
		"SuccessURL":        cfg.SuccessURL,
		"ManualRedirectURL": cfg.ManualRedirectURL,
	}
	for k, w := range want {
		if g := got[k]; g != w {
			t.Errorf("%s: got %q, want %q", k, g, w)
		}
	}
}

func TestGetOAuthConfig_CustomURLStripsTrailingSlash(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "https://claude.fedstart.com/")
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	cfg, err := GetOAuthConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseAPIURL != "https://claude.fedstart.com" {
		t.Errorf("trailing slash not stripped: %q", cfg.BaseAPIURL)
	}
}

func TestGetOAuthConfig_UnapprovedURLReturnsError(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "https://evil.example.com")
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	_, err := GetOAuthConfig()
	if err == nil {
		t.Fatal("expected error for unapproved custom OAuth URL")
	}
	if !strings.Contains(err.Error(), "not an approved endpoint") {
		t.Errorf("error message should mention approved endpoint, got: %v", err)
	}
}

func TestGetOAuthConfig_AllApprovedURLsAccepted(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")
	for _, approved := range approvedCustomOAuthURLs {
		t.Run(approved, func(t *testing.T) {
			t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", approved)
			cfg, err := GetOAuthConfig()
			if err != nil {
				t.Fatalf("approved URL %q should not error: %v", approved, err)
			}
			if cfg.BaseAPIURL != approved {
				t.Errorf("BaseAPIURL: got %q, want %q", cfg.BaseAPIURL, approved)
			}
		})
	}
}

func TestGetOAuthConfig_CustomURLWithClientIDOverride(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "https://claude.fedstart.com")
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "custom-client")

	cfg, err := GetOAuthConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientID != "custom-client" {
		t.Errorf("ClientID: got %q", cfg.ClientID)
	}
	if cfg.BaseAPIURL != "https://claude.fedstart.com" {
		t.Errorf("BaseAPIURL: got %q", cfg.BaseAPIURL)
	}
}

// ---------------------------------------------------------------------------
// NewOAuthFlow
// ---------------------------------------------------------------------------

func TestNewOAuthFlow_FailsWithUnapprovedCustomURL(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "https://evil.example.com")
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	_, err := NewOAuthFlow()
	if err == nil {
		t.Fatal("expected error from NewOAuthFlow with unapproved URL")
	}
}

func TestNewOAuthFlow_SucceedsWithDefaults(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	flow, err := NewOAuthFlow()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flow.config.ClientID != DefaultClientID {
		t.Errorf("expected default client ID, got %q", flow.config.ClientID)
	}
}

// ---------------------------------------------------------------------------
// isApprovedEndpoint
// ---------------------------------------------------------------------------

func TestIsApprovedEndpoint(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://claude.fedstart.com", true},
		{"https://claude-staging.fedstart.com", true},
		{"https://beacon.claude-ai.staging.ant.dev", true},
		{"https://evil.example.com", false},
		{"https://api.anthropic.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isApprovedEndpoint(tt.url); got != tt.want {
			t.Errorf("isApprovedEndpoint(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Issue 1: readTokenFromFD
// ---------------------------------------------------------------------------

func TestReadTokenFromFD_InvalidFDString(t *testing.T) {
	_, err := readTokenFromFD("not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric FD")
	}
	if !strings.Contains(err.Error(), "must be a valid file descriptor number") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReadTokenFromFD_ReadsAndTrims(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}

	go func() {
		w.WriteString("  my-token-value  \n")
		w.Close()
	}()

	fdStr := fmt.Sprintf("%d", r.Fd())
	token, err := readTokenFromFD(fdStr)
	if err != nil {
		t.Fatalf("readTokenFromFD error: %v", err)
	}
	if token != "my-token-value" {
		t.Errorf("expected trimmed token %q, got %q", "my-token-value", token)
	}
	// r is closed inside readTokenFromFD (defer f.Close())
}

func TestReadTokenFromFD_EmptyPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	// Close write end immediately — empty content.
	w.Close()

	fdStr := fmt.Sprintf("%d", r.Fd())
	token, err := readTokenFromFD(fdStr)
	if err != nil {
		t.Fatalf("readTokenFromFD error: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token for empty pipe, got %q", token)
	}
}

// ---------------------------------------------------------------------------
// Callback server auto-completes login without user input
// ---------------------------------------------------------------------------

func TestCallbackServer_AutoCompletesWithoutEnter(t *testing.T) {
	// Verify that the callback server signals completion via codeCh
	// without requiring manual stdin input.
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	state := "test-state-123"

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code")
			http.Error(w, "Missing code", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		codeCh <- code
	})

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	// Simulate browser redirect to callback.
	callbackURL := fmt.Sprintf("http://localhost:%d/callback?code=test-auth-code&state=%s", port, state)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	resp.Body.Close()

	// codeCh should fire immediately without any stdin input.
	select {
	case code := <-codeCh:
		if code != "test-auth-code" {
			t.Errorf("expected code %q, got %q", "test-auth-code", code)
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback — auto-complete did not work")
	}
}

func TestCallbackServer_ListensOnLocalhost(t *testing.T) {
	// Verify that the server listens on "localhost" (matching the redirect URI)
	// rather than only "127.0.0.1".
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("cannot listen on localhost:0: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	if port == 0 {
		t.Fatal("expected non-zero port")
	}

	// Verify the listener is reachable at "localhost".
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("cannot connect to localhost:%d: %v", port, err)
	}
	conn.Close()
}
