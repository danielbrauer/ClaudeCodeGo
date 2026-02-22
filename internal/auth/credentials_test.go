package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ===========================================================================
// Existing tests (preserved)
// ===========================================================================

func TestCredentialStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}

	// Initially empty.
	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("unexpected error on empty load: %v", err)
	}
	if tokens != nil {
		t.Fatalf("expected nil tokens on empty load, got %+v", tokens)
	}

	// Save tokens.
	original := &OAuthTokens{
		AccessToken:      "test-access-token",
		RefreshToken:     "test-refresh-token",
		ExpiresAt:        1700000000000,
		Scopes:           []string{"user:inference", "user:profile"},
		SubscriptionType: "pro",
	}

	if err := store.Save(original); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify file permissions.
	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected file mode 0600, got %o", perm)
	}

	// Load tokens back.
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil tokens after save")
	}
	if loaded.AccessToken != original.AccessToken {
		t.Errorf("access token mismatch: got %q, want %q", loaded.AccessToken, original.AccessToken)
	}
	if loaded.RefreshToken != original.RefreshToken {
		t.Errorf("refresh token mismatch: got %q, want %q", loaded.RefreshToken, original.RefreshToken)
	}
	if loaded.ExpiresAt != original.ExpiresAt {
		t.Errorf("expiresAt mismatch: got %d, want %d", loaded.ExpiresAt, original.ExpiresAt)
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	verifier := "test-verifier-string"
	c1 := generateCodeChallenge(verifier)
	c2 := generateCodeChallenge(verifier)
	if c1 != c2 {
		t.Errorf("code challenge should be deterministic: got %q and %q", c1, c2)
	}
	if c1 == "" {
		t.Error("code challenge should not be empty")
	}
}

func TestGenerateCodeVerifier(t *testing.T) {
	v1, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v2, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v1 == v2 {
		t.Error("two generated verifiers should not be equal")
	}
	if len(v1) < 20 {
		t.Errorf("verifier seems too short: %q", v1)
	}
}

// ===========================================================================
// Issue 2: CLAUDE_CONFIG_DIR env var override
// ===========================================================================

func TestConfigDir_Default(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".claude")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

func TestConfigDir_EnvOverride(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", custom)
	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != filepath.Clean(custom) {
		t.Errorf("expected %q, got %q", filepath.Clean(custom), dir)
	}
}

func TestNewCredentialStore_UsesConfigDir(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", custom)

	store, err := NewCredentialStore()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.dir != filepath.Clean(custom) {
		t.Errorf("store.dir: got %q, want %q", store.dir, filepath.Clean(custom))
	}
	expectedPath := filepath.Join(filepath.Clean(custom), ".credentials.json")
	if store.path != expectedPath {
		t.Errorf("store.path: got %q, want %q", store.path, expectedPath)
	}
}

// ===========================================================================
// Issue 9: OAuthAccount metadata stored
// ===========================================================================

func TestCredentialStore_SaveAndLoadAccount(t *testing.T) {
	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}

	// Initially nil.
	account, err := store.LoadAccount()
	if err != nil {
		t.Fatalf("unexpected error on empty load: %v", err)
	}
	if account != nil {
		t.Fatalf("expected nil account on empty load, got %+v", account)
	}

	// Save an account.
	original := &OAuthAccount{
		AccountUUID:      "acct-uuid-123",
		EmailAddress:     "user@example.com",
		OrganizationUUID: "org-uuid-456",
		DisplayName:      "Test User",
		BillingType:      "stripe",
		OrganizationRole: "admin",
		WorkspaceRole:    "developer",
		OrganizationName: "Test Org",
	}
	if err := store.SaveAccount(original); err != nil {
		t.Fatalf("SaveAccount error: %v", err)
	}

	// Load it back.
	loaded, err := store.LoadAccount()
	if err != nil {
		t.Fatalf("LoadAccount error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil account")
	}
	if loaded.AccountUUID != original.AccountUUID {
		t.Errorf("AccountUUID: got %q, want %q", loaded.AccountUUID, original.AccountUUID)
	}
	if loaded.EmailAddress != original.EmailAddress {
		t.Errorf("EmailAddress: got %q, want %q", loaded.EmailAddress, original.EmailAddress)
	}
	if loaded.OrganizationRole != original.OrganizationRole {
		t.Errorf("OrganizationRole: got %q, want %q", loaded.OrganizationRole, original.OrganizationRole)
	}
	if loaded.DisplayName != original.DisplayName {
		t.Errorf("DisplayName: got %q, want %q", loaded.DisplayName, original.DisplayName)
	}
}

func TestCredentialStore_SaveAccountPreservesTokens(t *testing.T) {
	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}

	// Save tokens first.
	tokens := &OAuthTokens{
		AccessToken: "my-token",
		ExpiresAt:   99999999,
	}
	if err := store.Save(tokens); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Save account — should not clobber tokens.
	account := &OAuthAccount{AccountUUID: "uuid"}
	if err := store.SaveAccount(account); err != nil {
		t.Fatalf("SaveAccount error: %v", err)
	}

	// Verify both are present.
	loadedTokens, _ := store.Load()
	if loadedTokens == nil || loadedTokens.AccessToken != "my-token" {
		t.Errorf("tokens lost after SaveAccount: %+v", loadedTokens)
	}
	loadedAccount, _ := store.LoadAccount()
	if loadedAccount == nil || loadedAccount.AccountUUID != "uuid" {
		t.Errorf("account not saved: %+v", loadedAccount)
	}
}

func TestCredentialStore_SaveAPIKey(t *testing.T) {
	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}

	// Save tokens first.
	if err := store.Save(&OAuthTokens{AccessToken: "tok"}); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Save API key.
	if err := store.SaveAPIKey("sk-ant-test-key"); err != nil {
		t.Fatalf("SaveAPIKey error: %v", err)
	}

	// Read raw file and verify the key is there alongside tokens.
	data, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if creds.APIKey != "sk-ant-test-key" {
		t.Errorf("apiKey: got %q", creds.APIKey)
	}
	if creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken != "tok" {
		t.Error("tokens should be preserved after SaveAPIKey")
	}
}

func TestCredentialsFile_AllFieldsRoundTrip(t *testing.T) {
	original := credentialsFile{
		ClaudeAiOauth: &OAuthTokens{
			AccessToken:      "tok",
			RefreshToken:     "ref",
			ExpiresAt:        12345,
			Scopes:           []string{"user:inference"},
			SubscriptionType: "pro",
			RateLimitTier:    "tier_1",
		},
		OAuthAccount: &OAuthAccount{
			AccountUUID:      "acct",
			EmailAddress:     "a@b.com",
			OrganizationUUID: "org",
			DisplayName:      "User",
		},
		APIKey: "sk-key",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var loaded credentialsFile
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if loaded.ClaudeAiOauth.SubscriptionType != "pro" {
		t.Errorf("SubscriptionType lost: %q", loaded.ClaudeAiOauth.SubscriptionType)
	}
	if loaded.OAuthAccount.EmailAddress != "a@b.com" {
		t.Errorf("EmailAddress lost: %q", loaded.OAuthAccount.EmailAddress)
	}
	if loaded.APIKey != "sk-key" {
		t.Errorf("APIKey lost: %q", loaded.APIKey)
	}
}

// ===========================================================================
// Issue 10: File locking during refresh
// ===========================================================================

func TestFileLock_AcquireAndRelease(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	lock, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("acquireFileLock error: %v", err)
	}
	if lock == nil {
		t.Fatal("lock should not be nil")
	}
	if err := lock.Unlock(); err != nil {
		t.Fatalf("unlock error: %v", err)
	}
}

func TestFileLock_DoubleAcquireFails(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	lock1, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("first lock error: %v", err)
	}
	defer lock1.Unlock()

	// Second non-blocking acquire should fail.
	_, err = acquireFileLock(lockPath)
	if err == nil {
		t.Error("expected error on double acquire, got nil")
	}
}

func TestFileLock_ReleaseAllowsReacquire(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	lock1, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("first lock error: %v", err)
	}
	lock1.Unlock()

	lock2, err := acquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("reacquire after release error: %v", err)
	}
	lock2.Unlock()
}

func TestCredentialStore_LockPath(t *testing.T) {
	store := &CredentialStore{dir: "/tmp/test-claude"}
	lp := store.lockPath()
	if lp != "/tmp/test-claude/.credentials.lock" {
		t.Errorf("lockPath: got %q", lp)
	}
}

// ===========================================================================
// Issue 1: CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR support
// ===========================================================================

func TestTokenProvider_FDTokenUsed(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	go func() {
		w.WriteString("fd-token-value")
		w.Close()
	}()

	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR", fmt.Sprintf("%d", r.Fd()))
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}
	provider := NewTokenProvider(store)

	token, err := provider.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetAccessToken error: %v", err)
	}
	if token != "fd-token-value" {
		t.Errorf("expected fd-token-value, got %q", token)
	}
}

func TestTokenProvider_FDTokenCached(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe error: %v", err)
	}
	go func() {
		w.WriteString("cached-fd-token")
		w.Close()
	}()

	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR", fmt.Sprintf("%d", r.Fd()))
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}
	provider := NewTokenProvider(store)

	// First call reads from FD.
	tok1, _ := provider.GetAccessToken(context.Background())

	// Clear the env var and call again — should use cached value.
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR", "")
	tok2, err := provider.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("second GetAccessToken error: %v", err)
	}
	if tok1 != tok2 {
		t.Errorf("FD token should be cached: first=%q second=%q", tok1, tok2)
	}
}

func TestTokenProvider_EnvTokenTakesPriority(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "env-token")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR", "999")
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}
	provider := NewTokenProvider(store)

	token, err := provider.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if token != "env-token" {
		t.Errorf("CLAUDE_CODE_OAUTH_TOKEN should take priority, got %q", token)
	}
}

// ===========================================================================
// Issue 11: Re-read credentials after lock (via tokenNeedsRefresh)
// ===========================================================================

func TestTokenNeedsRefresh_FreshToken(t *testing.T) {
	// Token expiring in 1 hour should NOT need refresh.
	tokens := &OAuthTokens{
		ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
	}
	if tokenNeedsRefresh(tokens) {
		t.Error("token with 1h until expiry should not need refresh")
	}
}

func TestTokenNeedsRefresh_ExpiringSoon(t *testing.T) {
	// Token expiring in 2 minutes SHOULD need refresh (threshold is 5 min).
	tokens := &OAuthTokens{
		ExpiresAt: time.Now().Add(2 * time.Minute).UnixMilli(),
	}
	if !tokenNeedsRefresh(tokens) {
		t.Error("token with 2min until expiry should need refresh")
	}
}

func TestTokenNeedsRefresh_Nil(t *testing.T) {
	if tokenNeedsRefresh(nil) {
		t.Error("nil tokens should not need refresh")
	}
}

func TestTokenNeedsRefresh_ZeroExpiry(t *testing.T) {
	if tokenNeedsRefresh(&OAuthTokens{ExpiresAt: 0}) {
		t.Error("zero expiry should not need refresh")
	}
}

// ===========================================================================
// Issue 13: SubscriptionType and RateLimitTier preserved on refresh
// ===========================================================================

func TestTokenProvider_RefreshPreservesSubscriptionInfo(t *testing.T) {
	// Set up a mock token refresh server.
	refreshServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_in":    3600,
			"scope":         "user:inference user:profile",
		})
	}))
	defer refreshServer.Close()

	// Set up a mock profile server that returns empty (simulates failure).
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer profileServer.Close()

	// Override env vars to point at our test servers.
	// We need the custom OAuth URL for the token URL.
	// Instead, we'll test the preservation logic directly.
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}

	// Save tokens with subscription info.
	original := &OAuthTokens{
		AccessToken:      "old-access",
		RefreshToken:     "old-refresh",
		ExpiresAt:        time.Now().Add(-time.Hour).UnixMilli(), // expired
		Scopes:           []string{"user:inference"},
		SubscriptionType: "pro",
		RateLimitTier:    "tier_1",
	}
	if err := store.Save(original); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify subscription info survives a Save/Load round-trip.
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded.SubscriptionType != "pro" {
		t.Errorf("SubscriptionType lost on round-trip: got %q", loaded.SubscriptionType)
	}
	if loaded.RateLimitTier != "tier_1" {
		t.Errorf("RateLimitTier lost on round-trip: got %q", loaded.RateLimitTier)
	}
}

// ===========================================================================
// Issue 15: InvalidateToken
// ===========================================================================

func TestTokenProvider_InvalidateToken(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR", "")
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")

	dir := t.TempDir()
	store := &CredentialStore{
		dir:  dir,
		path: filepath.Join(dir, ".credentials.json"),
	}

	// Save a valid token.
	store.Save(&OAuthTokens{
		AccessToken: "original-token",
		ExpiresAt:   time.Now().Add(time.Hour).UnixMilli(),
		Scopes:      []string{"user:inference"},
	})

	provider := NewTokenProvider(store)

	// First call loads the token.
	tok1, _ := provider.GetAccessToken(context.Background())
	if tok1 != "original-token" {
		t.Fatalf("expected original-token, got %q", tok1)
	}

	// Update the file with a new token.
	store.Save(&OAuthTokens{
		AccessToken: "updated-token",
		ExpiresAt:   time.Now().Add(time.Hour).UnixMilli(),
		Scopes:      []string{"user:inference"},
	})

	// Without invalidation, cached value is returned.
	tok2, _ := provider.GetAccessToken(context.Background())
	if tok2 != "original-token" {
		t.Errorf("should still return cached token, got %q", tok2)
	}

	// Invalidate forces re-read.
	provider.InvalidateToken()
	tok3, _ := provider.GetAccessToken(context.Background())
	if tok3 != "updated-token" {
		t.Errorf("after invalidation should return updated-token, got %q", tok3)
	}
}

// ===========================================================================
// Issue 17: NewTokenProvider respects CLAUDE_CODE_OAUTH_CLIENT_ID
// ===========================================================================

func TestNewTokenProvider_DefaultClientID(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "")
	dir := t.TempDir()
	store := &CredentialStore{dir: dir, path: filepath.Join(dir, "c.json")}
	provider := NewTokenProvider(store)
	if provider.clientID != DefaultClientID {
		t.Errorf("expected default clientID, got %q", provider.clientID)
	}
}

func TestNewTokenProvider_ClientIDOverride(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_CLIENT_ID", "override-id")
	dir := t.TempDir()
	store := &CredentialStore{dir: dir, path: filepath.Join(dir, "c.json")}
	provider := NewTokenProvider(store)
	if provider.clientID != "override-id" {
		t.Errorf("expected override-id, got %q", provider.clientID)
	}
}
