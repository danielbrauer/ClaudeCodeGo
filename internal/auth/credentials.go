package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// credentialsFile is the JSON structure stored in ~/.claude/.credentials.json.
type credentialsFile struct {
	ClaudeAiOauth *OAuthTokens `json:"claudeAiOauth,omitempty"`
}

// CredentialStore manages OAuth credential persistence.
type CredentialStore struct {
	mu   sync.Mutex
	dir  string
	path string
}

// NewCredentialStore creates a store using the default ~/.claude/ directory.
func NewCredentialStore() (*CredentialStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	dir := filepath.Join(home, ".claude")
	return &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}, nil
}

// Load reads OAuth tokens from the credentials file.
func (s *CredentialStore) Load() (*OAuthTokens, error) {
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

	return creds.ClaudeAiOauth, nil
}

// Save writes OAuth tokens to the credentials file.
func (s *CredentialStore) Save(tokens *OAuthTokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

// TokenProvider manages token retrieval and automatic refresh.
type TokenProvider struct {
	store    *CredentialStore
	clientID string
	mu       sync.Mutex
	cached   *OAuthTokens
}

// NewTokenProvider creates a token provider with the given credential store.
func NewTokenProvider(store *CredentialStore) *TokenProvider {
	return &TokenProvider{
		store:    store,
		clientID: DefaultClientID,
	}
}

// GetAccessToken returns a valid access token, refreshing if necessary.
func (p *TokenProvider) GetAccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check environment variable first.
	if envToken := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); envToken != "" {
		return envToken, nil
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

func (p *TokenProvider) refresh(ctx context.Context) error {
	if p.cached.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	resp, err := RefreshAccessToken(ctx, p.cached.RefreshToken, p.clientID)
	if err != nil {
		return err
	}

	refreshToken := resp.RefreshToken
	if refreshToken == "" {
		refreshToken = p.cached.RefreshToken
	}

	p.cached = &OAuthTokens{
		AccessToken:  resp.AccessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).UnixMilli(),
		Scopes:       strings.Split(resp.Scope, " "),
	}

	// Persist refreshed tokens.
	if err := p.store.Save(p.cached); err != nil {
		// Log but don't fail - we still have a valid token in memory.
		fmt.Fprintf(os.Stderr, "Warning: failed to save refreshed tokens: %v\n", err)
	}

	return nil
}
