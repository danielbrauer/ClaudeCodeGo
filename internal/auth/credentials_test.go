package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialStore_SaveAndLoad(t *testing.T) {
	// Use a temp directory.
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
	// Verify code challenge is deterministic.
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
