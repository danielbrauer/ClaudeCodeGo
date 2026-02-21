// Package auth implements OAuth authentication for Claude subscription accounts.
package auth

import (
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
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// OAuth configuration constants (extracted from cli.js).
const (
	DefaultClientID    = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	DefaultBaseAPIURL  = "https://api.anthropic.com"
	AuthorizeURL       = "https://claude.ai/oauth/authorize"
	TokenURL           = "https://platform.claude.com/v1/oauth/token"
	ManualRedirectURL  = "https://platform.claude.com/oauth/code/callback"
	SuccessURL         = "https://platform.claude.com/oauth/code/success?app=claude-code"
	OAuthVersion       = "oauth-2025-04-20"
)

// Scopes requested during authentication.
var DefaultScopes = []string{
	"user:profile",
	"user:inference",
	"user:sessions:claude_code",
	"user:mcp_servers",
	"org:create_api_key",
}

// TokenResponse is the response from the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
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
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// OAuthFlow manages the browser-based OAuth login.
type OAuthFlow struct {
	ClientID string
}

// NewOAuthFlow creates a new OAuthFlow with default settings.
func NewOAuthFlow() *OAuthFlow {
	return &OAuthFlow{ClientID: DefaultClientID}
}

// Login performs the full OAuth PKCE flow:
// 1. Start a local HTTP server for the callback
// 2. Open the browser to the authorization URL
// 3. Wait for the callback with the authorization code
// 4. Exchange the code for tokens
func (f *OAuthFlow) Login(ctx context.Context) (*OAuthTokens, error) {
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
		http.Redirect(w, r, SuccessURL, http.StatusFound)
		codeCh <- code
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer server.Shutdown(context.Background())

	// Build authorization URL.
	authURL := buildAuthURL(f.ClientID, challenge, state, port)

	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Could not open browser automatically: %v\n", err)
	}

	// Wait for the callback.
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("login timed out waiting for browser callback")
	}

	// Exchange code for tokens.
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)
	tokenResp, err := exchangeCode(ctx, code, verifier, state, redirectURI, f.ClientID)
	if err != nil {
		return nil, err
	}

	tokens := &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).UnixMilli(),
		Scopes:       strings.Split(tokenResp.Scope, " "),
	}

	return tokens, nil
}

func buildAuthURL(clientID, challenge, state string, port int) string {
	u, _ := url.Parse(AuthorizeURL)
	q := u.Query()
	q.Set("code", "true")
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", fmt.Sprintf("http://localhost:%d/callback", port))
	q.Set("scope", strings.Join(DefaultScopes, " "))
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}

func exchangeCode(ctx context.Context, code, verifier, state, redirectURI, clientID string) (*TokenResponse, error) {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  redirectURI,
		"client_id":     clientID,
		"code_verifier": verifier,
		"state":         state,
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", TokenURL, strings.NewReader(string(bodyJSON)))
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
func RefreshAccessToken(ctx context.Context, refreshToken, clientID string) (*TokenResponse, error) {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     clientID,
		"scope":         strings.Join(DefaultScopes, " "),
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", TokenURL, strings.NewReader(string(bodyJSON)))
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
