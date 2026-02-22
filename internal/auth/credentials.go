package auth

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OAuthTokens holds the stored OAuth credentials.
type OAuthTokens struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken"`
	ExpiresAt        int64    `json:"expiresAt"`
	Scopes           []string `json:"scopes"`
	SubscriptionType string   `json:"subscriptionType,omitempty"`
	RateLimitTier    string   `json:"rateLimitTier,omitempty"`
}

// credentialsFile is the JSON structure stored in {configDir}/.credentials.json.
// Issue 9: includes oauthAccount metadata and API key.
type credentialsFile struct {
	ClaudeAiOauth *OAuthTokens  `json:"claudeAiOauth,omitempty"`
	OAuthAccount  *OAuthAccount `json:"oauthAccount,omitempty"`
	APIKey        string        `json:"apiKey,omitempty"`
}

// ConfigDir returns the Claude configuration directory, respecting
// the CLAUDE_CONFIG_DIR environment variable override.
// Issue 2: CLAUDE_CONFIG_DIR support.
func ConfigDir() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// CredentialStore manages OAuth credential persistence.
type CredentialStore struct {
	mu   sync.Mutex
	dir  string
	path string
}

// NewCredentialStore creates a store using the config directory
// (CLAUDE_CONFIG_DIR or ~/.claude/).
func NewCredentialStore() (*CredentialStore, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	return &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}, nil
}

// Load reads OAuth tokens from the credentials file.
func (s *CredentialStore) Load() (*OAuthTokens, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// loadLocked reads tokens without acquiring the mutex (caller must hold it).
func (s *CredentialStore) loadLocked() (*OAuthTokens, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	return creds.ClaudeAiOauth, nil
}

// Save writes OAuth tokens to the credentials file.
func (s *CredentialStore) Save(tokens *OAuthTokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(tokens)
}

// saveLocked writes tokens without acquiring the mutex (caller must hold it).
func (s *CredentialStore) saveLocked(tokens *OAuthTokens) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("creating credentials directory: %w", err)
	}

	// Read existing file to preserve other fields.
	var creds credentialsFile
	data, err := os.ReadFile(s.path)
	if err == nil {
		json.Unmarshal(data, &creds) // ignore errors, overwrite
	}

	creds.ClaudeAiOauth = tokens

	newData, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}

	if err := os.WriteFile(s.path, newData, 0600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}

	return nil
}

// SaveAccount writes OAuth account metadata to the credentials file.
// Issue 9: store oauthAccount metadata.
func (s *CredentialStore) SaveAccount(account *OAuthAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("creating credentials directory: %w", err)
	}

	var creds credentialsFile
	data, err := os.ReadFile(s.path)
	if err == nil {
		json.Unmarshal(data, &creds)
	}

	creds.OAuthAccount = account

	newData, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}

	return os.WriteFile(s.path, newData, 0600)
}

// LoadAccount reads OAuth account metadata from the credentials file.
func (s *CredentialStore) LoadAccount() (*OAuthAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	return creds.OAuthAccount, nil
}

// SaveAPIKey writes the API key to the credentials file.
// Issue 8: store API key from create_api_key endpoint.
func (s *CredentialStore) SaveAPIKey(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("creating credentials directory: %w", err)
	}

	var creds credentialsFile
	data, err := os.ReadFile(s.path)
	if err == nil {
		json.Unmarshal(data, &creds)
	}

	creds.APIKey = key

	newData, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}

	return os.WriteFile(s.path, newData, 0600)
}

// lockPath returns the path to the credential lock file.
func (s *CredentialStore) lockPath() string {
	return filepath.Join(s.dir, ".credentials.lock")
}

// TokenProvider manages token retrieval and automatic refresh.
type TokenProvider struct {
	store    *CredentialStore
	clientID string
	mu       sync.Mutex
	cached   *OAuthTokens
	fdToken  string // cached token from file descriptor
	fdRead   bool   // whether we've attempted FD read
}

// NewTokenProvider creates a token provider with the given credential store.
// Issue 17: respects CLAUDE_CODE_OAUTH_CLIENT_ID override.
func NewTokenProvider(store *CredentialStore) *TokenProvider {
	clientID := DefaultClientID
	if envClientID := os.Getenv("CLAUDE_CODE_OAUTH_CLIENT_ID"); envClientID != "" {
		clientID = envClientID
	}
	return &TokenProvider{
		store:    store,
		clientID: clientID,
	}
}

// GetAccessToken returns a valid access token, refreshing if necessary.
// Issue 1: supports CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR.
func (p *TokenProvider) GetAccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check CLAUDE_CODE_OAUTH_TOKEN environment variable first.
	if envToken := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); envToken != "" {
		return envToken, nil
	}

	// Issue 1: Check CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR.
	if !p.fdRead {
		p.fdRead = true
		if fdStr := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR"); fdStr != "" {
			token, err := readTokenFromFD(fdStr)
			if err != nil {
				return "", err
			}
			if token != "" {
				p.fdToken = token
			}
		}
	}
	if p.fdToken != "" {
		return p.fdToken, nil
	}

	// Load from store if not cached.
	if p.cached == nil {
		tokens, err := p.store.Load()
		if err != nil {
			return "", fmt.Errorf("loading tokens: %w", err)
		}
		if tokens == nil {
			return "", fmt.Errorf("not authenticated - run 'claude' to log in")
		}
		p.cached = tokens
	}

	// Check if token needs refresh (refresh if <5 minutes until expiry).
	if p.needsRefresh() {
		if err := p.refresh(ctx); err != nil {
			return "", fmt.Errorf("refreshing token: %w", err)
		}
	}

	return p.cached.AccessToken, nil
}

// InvalidateToken clears the cached token, forcing a reload on next access.
// Used by the API client for 401 auto-retry (Issue 15).
func (p *TokenProvider) InvalidateToken() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cached = nil
}

// HasValidScopes returns true if the current token has inference scopes.
func (p *TokenProvider) HasValidScopes() bool {
	if p.cached == nil {
		return false
	}
	for _, s := range p.cached.Scopes {
		if s == "user:inference" {
			return true
		}
	}
	return false
}

func (p *TokenProvider) needsRefresh() bool {
	if p.cached == nil || p.cached.ExpiresAt == 0 {
		return false
	}
	// Refresh if less than 5 minutes until expiry.
	return time.Now().UnixMilli() > p.cached.ExpiresAt-(5*time.Minute).Milliseconds()
}

// tokenNeedsRefresh checks if a given token set needs refresh.
func tokenNeedsRefresh(tokens *OAuthTokens) bool {
	if tokens == nil || tokens.ExpiresAt == 0 {
		return false
	}
	return time.Now().UnixMilli() > tokens.ExpiresAt-(5*time.Minute).Milliseconds()
}

// refresh performs token refresh with file locking and profile re-fetch.
// Issues 10, 11, 12, 13: file locking, re-read after lock, profile re-fetch,
// preserve subscription info.
func (p *TokenProvider) refresh(ctx context.Context) error {
	if p.cached.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	// Get OAuth config for token URL and base API URL.
	cfg, err := GetOAuthConfig()
	if err != nil {
		return fmt.Errorf("getting OAuth config: %w", err)
	}

	// Issue 10: Acquire file lock with retry (up to 5 retries, 1-2s backoff).
	lockPath := p.store.lockPath()
	if err := os.MkdirAll(p.store.dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	var lock *fileLock
	for attempt := 0; attempt <= 5; attempt++ {
		lock, err = acquireFileLock(lockPath)
		if err == nil {
			break
		}
		if attempt == 5 {
			return fmt.Errorf("could not acquire credential lock after 5 retries: %w", err)
		}
		// Sleep 1-2s with random jitter.
		var jitterBytes [8]byte
		rand.Read(jitterBytes[:])
		jitter := time.Duration(binary.LittleEndian.Uint64(jitterBytes[:]) % uint64(time.Second))
		time.Sleep(time.Second + jitter)
	}
	defer lock.Unlock()

	// Issue 11: Re-read credentials after acquiring lock to check if another
	// process already refreshed the token.
	freshTokens, err := p.store.Load()
	if err == nil && freshTokens != nil && !tokenNeedsRefresh(freshTokens) {
		p.cached = freshTokens
		return nil
	}

	// Perform the actual refresh.
	resp, err := RefreshAccessToken(ctx, p.cached.RefreshToken, p.clientID, cfg.TokenURL)
	if err != nil {
		return err
	}

	refreshToken := resp.RefreshToken
	if refreshToken == "" {
		refreshToken = p.cached.RefreshToken
	}

	// Issue 12: Fetch profile info after refresh to update subscription metadata.
	profileInfo, profileErr := FetchProfileInfo(ctx, cfg.BaseAPIURL, resp.AccessToken)
	if profileErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch profile after refresh: %v\n", profileErr)
		profileInfo = &ProfileInfo{}
	}

	// Issue 13: Preserve subscription info — use profile data for the new token.
	// Falls back to previous values if profile fetch failed.
	subscriptionType := profileInfo.SubscriptionType
	rateLimitTier := profileInfo.RateLimitTier
	if subscriptionType == "" && p.cached != nil {
		subscriptionType = p.cached.SubscriptionType
	}
	if rateLimitTier == "" && p.cached != nil {
		rateLimitTier = p.cached.RateLimitTier
	}

	p.cached = &OAuthTokens{
		AccessToken:      resp.AccessToken,
		RefreshToken:     refreshToken,
		ExpiresAt:        time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).UnixMilli(),
		Scopes:           strings.Split(resp.Scope, " "),
		SubscriptionType: subscriptionType,
		RateLimitTier:    rateLimitTier,
	}

	// Persist refreshed tokens.
	if err := p.store.Save(p.cached); err != nil {
		// Log but don't fail — we still have a valid token in memory.
		fmt.Fprintf(os.Stderr, "Warning: failed to save refreshed tokens: %v\n", err)
	}

	// Issue 12: Update stored account metadata from profile refresh.
	if profileErr == nil {
		account, _ := p.store.LoadAccount()
		if account != nil {
			updated := false
			if profileInfo.DisplayName != "" {
				account.DisplayName = profileInfo.DisplayName
				updated = true
			}
			if profileInfo.HasExtraUsageEnabled {
				account.HasExtraUsageEnabled = profileInfo.HasExtraUsageEnabled
				updated = true
			}
			if profileInfo.BillingType != "" {
				account.BillingType = profileInfo.BillingType
				updated = true
			}
			if updated {
				if err := p.store.SaveAccount(account); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to update account metadata: %v\n", err)
				}
			}
		}
	}

	return nil
}

// readTokenFromFD reads an OAuth token from a file descriptor number.
// Issue 1: CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR support.
func readTokenFromFD(fdStr string) (string, error) {
	fdNum, err := strconv.Atoi(fdStr)
	if err != nil {
		return "", fmt.Errorf("CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR must be a valid file descriptor number, got: %s", fdStr)
	}
	f := os.NewFile(uintptr(fdNum), "oauth-token-fd")
	if f == nil {
		return "", fmt.Errorf("invalid file descriptor: %d", fdNum)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("reading token from file descriptor %d: %w", fdNum, err)
	}

	return strings.TrimSpace(string(data)), nil
}
