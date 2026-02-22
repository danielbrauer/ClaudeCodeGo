package config

import (
	"context"
	"encoding/json"
	"testing"
)

// mockFallbackHandler always returns a specific value for testing.
type mockFallbackHandler struct {
	allow bool
}

func (h *mockFallbackHandler) RequestPermission(_ context.Context, _ string, _ json.RawMessage) (bool, error) {
	return h.allow, nil
}

func TestRuleBasedPermissionHandlerAllow(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "Bash", Pattern: "npm run *", Action: "allow"},
	}
	handler := NewRuleBasedPermissionHandler(rules, &mockFallbackHandler{allow: false})

	input := json.RawMessage(`{"command": "npm run test"}`)
	allowed, err := handler.RequestPermission(context.Background(), "Bash", input)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if !allowed {
		t.Error("expected allowed for matching allow rule")
	}
}

func TestRuleBasedPermissionHandlerDeny(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "FileRead", Pattern: ".env", Action: "deny"},
	}
	handler := NewRuleBasedPermissionHandler(rules, &mockFallbackHandler{allow: true})

	input := json.RawMessage(`{"file_path": ".env"}`)
	allowed, err := handler.RequestPermission(context.Background(), "FileRead", input)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if allowed {
		t.Error("expected denied for matching deny rule")
	}
}

func TestRuleBasedPermissionHandlerFallback(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "Bash", Pattern: "npm *", Action: "allow"},
	}
	// Fallback should allow.
	handler := NewRuleBasedPermissionHandler(rules, &mockFallbackHandler{allow: true})

	// Non-matching command should fall through to fallback.
	input := json.RawMessage(`{"command": "rm -rf /"}`)
	allowed, err := handler.RequestPermission(context.Background(), "Bash", input)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if !allowed {
		t.Error("expected fallback to be used (allow)")
	}
}

func TestRuleBasedPermissionHandlerToolMismatch(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "Bash", Action: "allow"},
	}
	handler := NewRuleBasedPermissionHandler(rules, &mockFallbackHandler{allow: false})

	// FileWrite should not match a Bash rule.
	input := json.RawMessage(`{"file_path": "/tmp/test"}`)
	allowed, err := handler.RequestPermission(context.Background(), "FileWrite", input)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if allowed {
		t.Error("expected denied for tool mismatch")
	}
}

func TestRuleBasedPermissionHandlerNoPattern(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "Bash", Action: "allow"},
	}
	handler := NewRuleBasedPermissionHandler(rules, &mockFallbackHandler{allow: false})

	// Rule with no pattern should match all Bash calls.
	input := json.RawMessage(`{"command": "anything"}`)
	allowed, err := handler.RequestPermission(context.Background(), "Bash", input)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if !allowed {
		t.Error("expected allowed for pattern-less rule")
	}
}

func TestRuleBasedPermissionHandlerFirstMatchWins(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "Bash", Pattern: "npm *", Action: "deny"},
		{Tool: "Bash", Action: "allow"},
	}
	handler := NewRuleBasedPermissionHandler(rules, &mockFallbackHandler{allow: true})

	// "npm test" should match the first deny rule.
	input := json.RawMessage(`{"command": "npm test"}`)
	allowed, err := handler.RequestPermission(context.Background(), "Bash", input)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if allowed {
		t.Error("expected denied: first matching rule is deny")
	}

	// "ls" should match the second allow rule.
	input2 := json.RawMessage(`{"command": "ls"}`)
	allowed2, err := handler.RequestPermission(context.Background(), "Bash", input2)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if !allowed2 {
		t.Error("expected allowed: second rule matches all Bash")
	}
}

func TestRuleBasedPermissionHandlerWebFetchDomain(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "WebFetch", Pattern: "domain:example.com", Action: "allow"},
	}
	handler := NewRuleBasedPermissionHandler(rules, &mockFallbackHandler{allow: false})

	input := json.RawMessage(`{"url": "https://example.com/api/data"}`)
	allowed, err := handler.RequestPermission(context.Background(), "WebFetch", input)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if !allowed {
		t.Error("expected allowed for matching domain")
	}

	// Non-matching domain.
	input2 := json.RawMessage(`{"url": "https://other.com/api/data"}`)
	allowed2, err := handler.RequestPermission(context.Background(), "WebFetch", input2)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if allowed2 {
		t.Error("expected denied for non-matching domain")
	}
}

func TestRuleBasedPermissionHandlerFilePathGlob(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "FileRead", Pattern: "*.env", Action: "deny"},
	}
	handler := NewRuleBasedPermissionHandler(rules, &mockFallbackHandler{allow: true})

	input := json.RawMessage(`{"file_path": ".env"}`)
	allowed, err := handler.RequestPermission(context.Background(), "FileRead", input)
	if err != nil {
		t.Fatalf("RequestPermission: %v", err)
	}
	if allowed {
		t.Error("expected denied for .env file matching *.env pattern")
	}
}

func TestMatchPatternGlob(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"npm run *", "npm run test", true},
		{"npm run *", "npm install", false},
		{"*.env", ".env", true},
		{"*.env", "production.env", true},
		{"*.go", "main.go", true},
		{"domain:example.com", "https://example.com/path", true},
		{"domain:example.com", "https://other.com/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.value, func(t *testing.T) {
			got := matchPattern(tt.pattern, tt.value)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
			}
		})
	}
}
