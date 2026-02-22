// Package auth implements OAuth authentication for Claude subscription accounts.
package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Default OAuth configuration constants (extracted from cli.js).
const (
	DefaultClientID       = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	DefaultBaseAPIURL     = "https://api.anthropic.com"
	DefaultAuthorizeURL   = "https://claude.ai/oauth/authorize"
	DefaultTokenURL       = "https://platform.claude.com/v1/oauth/token"
	DefaultManualRedirect = "https://platform.claude.com/oauth/code/callback"
	DefaultSuccessURL     = "https://platform.claude.com/oauth/code/success?app=claude-code"
	DefaultAPIKeyURL      = "https://api.anthropic.com/api/oauth/claude_cli/create_api_key"
	DefaultRolesURL       = "https://api.anthropic.com/api/oauth/claude_cli/roles"
	OAuthVersion          = "oauth-2025-04-20"
)

// approvedCustomOAuthURLs is the allowlist for CLAUDE_CODE_CUSTOM_OAUTH_URL.
// Matches the JS version's approved endpoint list.
var approvedCustomOAuthURLs = []string{
	"https://beacon.claude-ai.staging.ant.dev",
	"https://claude.fedstart.com",
	"https://claude-staging.fedstart.com",
}

// Scopes requested during authentication.
var DefaultScopes = []string{
	"user:profile",
	"user:inference",
	"user:sessions:claude_code",
	"user:mcp_servers",
	"org:create_api_key",
}

// OAuthURLConfig holds all OAuth-related URLs, supporting env var overrides.
type OAuthURLConfig struct {
	BaseAPIURL        string
	AuthorizeURL      string
	TokenURL          string
	APIKeyURL         string
	RolesURL          string
	SuccessURL        string
	ManualRedirectURL string
	ClientID          string
}

// GetOAuthConfig builds the OAuth URL configuration from defaults and env var overrides.
// Issue 17: CLAUDE_CODE_OAUTH_CLIENT_ID override.
// Issue 18: CLAUDE_CODE_CUSTOM_OAUTH_URL override.
func GetOAuthConfig() (*OAuthURLConfig, error) {
	cfg := &OAuthURLConfig{
		BaseAPIURL:        DefaultBaseAPIURL,
		AuthorizeURL:      DefaultAuthorizeURL,
		TokenURL:          DefaultTokenURL,
		APIKeyURL:         DefaultAPIKeyURL,
		RolesURL:          DefaultRolesURL,
		SuccessURL:        DefaultSuccessURL,
		ManualRedirectURL: DefaultManualRedirect,
		ClientID:          DefaultClientID,
	}

	if customURL := os.Getenv("CLAUDE_CODE_CUSTOM_OAUTH_URL"); customURL != "" {
		customURL = strings.TrimRight(customURL, "/")
		if !isApprovedEndpoint(customURL) {
			return nil, fmt.Errorf("CLAUDE_CODE_CUSTOM_OAUTH_URL is not an approved endpoint")
		}
		cfg.BaseAPIURL = customURL
		cfg.AuthorizeURL = customURL + "/oauth/authorize"
		cfg.TokenURL = customURL + "/v1/oauth/token"
		cfg.APIKeyURL = customURL + "/api/oauth/claude_cli/create_api_key"
		cfg.RolesURL = customURL + "/api/oauth/claude_cli/roles"
		cfg.SuccessURL = customURL + "/oauth/code/success?app=claude-code"
		cfg.ManualRedirectURL = customURL + "/oauth/code/callback"
	}

	if clientID := os.Getenv("CLAUDE_CODE_OAUTH_CLIENT_ID"); clientID != "" {
		cfg.ClientID = clientID
	}

	return cfg, nil
}

func isApprovedEndpoint(url string) bool {
	for _, approved := range approvedCustomOAuthURLs {
		if url == approved {
			return true
		}
	}
	return false
}

// TokenResponse is the response from the token endpoint.
// Issue 5: includes account and organization fields.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	Account      *struct {
		UUID         string `json:"uuid"`
		EmailAddress string `json:"email_address"`
	} `json:"account,omitempty"`
	Organization *struct {
		UUID string `json:"uuid"`
	} `json:"organization,omitempty"`
}

// LoginResult holds the full result of an OAuth login flow.
type LoginResult struct {
	Tokens  *OAuthTokens
	Account *OAuthAccount
	APIKey  string
}

// generateCodeVerifier creates a random PKCE code verifier.
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating code verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge computes the S256 code challenge from a verifier.
func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// generateState creates a random state parameter.
// Issue 3: uses 32 random bytes (matching JS randomBytes(32)).
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// OAuthFlow manages the browser-based OAuth login.
type OAuthFlow struct {
	config *OAuthURLConfig
}

// NewOAuthFlow creates a new OAuthFlow with configuration from defaults and env vars.
func NewOAuthFlow() (*OAuthFlow, error) {
	cfg, err := GetOAuthConfig()
	if err != nil {
		return nil, err
	}
	return &OAuthFlow{config: cfg}, nil
}

// Login performs the full OAuth PKCE flow:
// 1. Start a local HTTP server for the callback
// 2. Open the browser to the authorization URL
// 3. Wait for the callback with the authorization code (or manual entry)
// 4. Exchange the code for tokens
// 5. Fetch profile info, roles, and create API key
func (f *OAuthFlow) Login(ctx context.Context) (*LoginResult, error) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, err
	}
	challenge := generateCodeChallenge(verifier)

	state, err := generateState()
	if err != nil {
		return nil, err
	}

	// Start local callback server.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errCh <- fmt.Errorf("OAuth error: %s: %s", errMsg, r.URL.Query().Get("error_description"))
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no authorization code in callback")
			http.Error(w, "Missing code", http.StatusBadRequest)
			return
		}
		// Redirect browser to success page.
		http.Redirect(w, r, f.config.SuccessURL, http.StatusFound)
		codeCh <- code
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer server.Shutdown(context.Background())

	// Issue 4: Build both automatic and manual authorization URLs.
	autoAuthURL := buildAuthURL(f.config, challenge, state, port, false)
	manualAuthURL := buildAuthURL(f.config, challenge, state, port, true)

	fmt.Println("Opening browser for authentication...")
	if err := openBrowser(autoAuthURL); err != nil {
		fmt.Printf("Could not open browser automatically: %v\n", err)
	}
	fmt.Printf("\nIf the browser doesn't open, visit this URL on this machine:\n%s\n\n", autoAuthURL)
	fmt.Printf("Or visit this URL on another device and paste the code below:\n%s\n\n", manualAuthURL)

	// Start goroutine to read manual code entry from stdin.
	manualCodeCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text != "" {
				manualCodeCh <- text
				return
			}
		}
	}()

	// Wait for the callback, manual code entry, or timeout.
	var code string
	isManual := false
	select {
	case code = <-codeCh:
		// Browser callback succeeded.
	case code = <-manualCodeCh:
		isManual = true
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("login timed out waiting for browser callback")
	}

	// The manual code page shows the code in the format "authorizationCode#state".
	// Split on "#" to extract just the authorization code.
	if isManual {
		parts := strings.SplitN(code, "#", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid code format: expected code#state — make sure the full code was copied")
		}
		code = parts[0]
		// Validate the state from the pasted code matches what we generated.
		if parts[1] != state {
			return nil, fmt.Errorf("state mismatch in manual code entry")
		}
	}

	// Exchange code for tokens.
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)
	if isManual {
		redirectURI = f.config.ManualRedirectURL
	}
	tokenResp, err := exchangeCode(ctx, code, verifier, state, redirectURI, f.config.ClientID, f.config.TokenURL)
	if err != nil {
		return nil, err
	}

	tokens := &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).UnixMilli(),
		Scopes:       strings.Split(tokenResp.Scope, " "),
	}

	// Issue 6: Fetch profile info to populate subscription data.
	profileInfo, err := FetchProfileInfo(ctx, f.config.BaseAPIURL, tokenResp.AccessToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch profile: %v\n", err)
		profileInfo = &ProfileInfo{}
	}

	tokens.SubscriptionType = profileInfo.SubscriptionType
	tokens.RateLimitTier = profileInfo.RateLimitTier

	// Issue 5 & 9: Build account metadata from token response and profile.
	var account *OAuthAccount
	if tokenResp.Account != nil {
		account = &OAuthAccount{
			AccountUUID:          tokenResp.Account.UUID,
			EmailAddress:         tokenResp.Account.EmailAddress,
			DisplayName:          profileInfo.DisplayName,
			HasExtraUsageEnabled: profileInfo.HasExtraUsageEnabled,
			BillingType:          profileInfo.BillingType,
		}
		if tokenResp.Organization != nil {
			account.OrganizationUUID = tokenResp.Organization.UUID
		}
	}

	// Issue 7: Fetch and store roles.
	if account != nil {
		roles, err := FetchRoles(ctx, f.config.RolesURL, tokenResp.AccessToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch roles: %v\n", err)
		} else {
			account.OrganizationRole = roles.OrganizationRole
			account.WorkspaceRole = roles.WorkspaceRole
			account.OrganizationName = roles.OrganizationName
		}
	}

	// Issue 8: Create and store API key.
	apiKey, _ := CreateAPIKey(ctx, f.config.APIKeyURL, tokenResp.AccessToken)

	return &LoginResult{
		Tokens:  tokens,
		Account: account,
		APIKey:  apiKey,
	}, nil
}

// Issue 4: buildAuthURL accepts isManual to switch between localhost and platform redirect.
func buildAuthURL(cfg *OAuthURLConfig, challenge, state string, port int, isManual bool) string {
	u, _ := url.Parse(cfg.AuthorizeURL)
	q := u.Query()
	q.Set("code", "true")
	q.Set("client_id", cfg.ClientID)
	q.Set("response_type", "code")
	if isManual {
		q.Set("redirect_uri", cfg.ManualRedirectURL)
	} else {
		q.Set("redirect_uri", fmt.Sprintf("http://localhost:%d/callback", port))
	}
	q.Set("scope", strings.Join(DefaultScopes, " "))
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}

func exchangeCode(ctx context.Context, code, verifier, state, redirectURI, clientID, tokenURL string) (*TokenResponse, error) {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  redirectURI,
		"client_id":     clientID,
		"code_verifier": verifier,
		"state":         state,
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode == 401 {
			return nil, fmt.Errorf("authentication failed: invalid authorization code")
		}
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshAccessToken refreshes an OAuth access token using the refresh token.
func RefreshAccessToken(ctx context.Context, refreshToken, clientID, tokenURL string) (*TokenResponse, error) {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     clientID,
		"scope":         strings.Join(DefaultScopes, " "),
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, fmt.Errorf("creating refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading refresh response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing refresh response: %w", err)
	}

	return &tokenResp, nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}
