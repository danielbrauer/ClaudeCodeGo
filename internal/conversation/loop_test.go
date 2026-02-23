package conversation

import (
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
)

func TestLoop_Clear(t *testing.T) {
	history := NewHistory()
	history.AddUserMessage("hello")
	history.AddUserMessage("world")

	loop := NewLoop(LoopConfig{
		History: history,
	})

	if loop.History().Len() != 2 {
		t.Fatalf("before clear: Len = %d, want 2", loop.History().Len())
	}

	loop.Clear()

	if loop.History().Len() != 0 {
		t.Errorf("after clear: Len = %d, want 0", loop.History().Len())
	}
	if loop.History().Messages() != nil {
		t.Errorf("after clear: Messages should be nil, got %v", loop.History().Messages())
	}
}

func TestLoop_ClearEmptyHistory(t *testing.T) {
	loop := NewLoop(LoopConfig{})

	// Clear on empty history should be a no-op.
	loop.Clear()

	if loop.History().Len() != 0 {
		t.Errorf("Len = %d, want 0", loop.History().Len())
	}
}

func TestLoop_ClearThenAddMessages(t *testing.T) {
	history := NewHistory()
	history.AddUserMessage("old message")

	loop := NewLoop(LoopConfig{
		History: history,
	})

	loop.Clear()

	// After clear, new messages should work normally.
	loop.History().AddUserMessage("new message")

	if loop.History().Len() != 1 {
		t.Fatalf("Len = %d, want 1", loop.History().Len())
	}

	msg := loop.History().Messages()[0]
	if msg.Role != api.RoleUser {
		t.Errorf("Role = %q, want %q", msg.Role, api.RoleUser)
	}
}

func TestLoop_SetOnTurnComplete(t *testing.T) {
	called := false
	loop := NewLoop(LoopConfig{
		OnTurnComplete: func(h *History) {
			t.Error("original callback should not be called")
		},
	})

	// Replace the callback.
	loop.SetOnTurnComplete(func(h *History) {
		called = true
	})

	// Trigger notifyTurnComplete indirectly by checking the field was set.
	// Since notifyTurnComplete is unexported, we verify via History() and
	// the fact that the loop was constructed with the new callback.
	// We can test this by adding a message and verifying the callback fires.
	loop.History().AddUserMessage("test")

	// Call the internal notify method via the exported Clear + manual check.
	// Since we can't call notifyTurnComplete directly, verify the setter worked
	// by reading back the behavior.
	if loop.onTurnComplete == nil {
		t.Fatal("onTurnComplete should not be nil after SetOnTurnComplete")
	}

	loop.onTurnComplete(loop.History())
	if !called {
		t.Error("replacement callback was not called")
	}
}

func TestLoop_SetOnTurnCompleteNil(t *testing.T) {
	loop := NewLoop(LoopConfig{
		OnTurnComplete: func(h *History) {
			t.Error("should not be called")
		},
	})

	loop.SetOnTurnComplete(nil)

	if loop.onTurnComplete != nil {
		t.Error("expected onTurnComplete to be nil")
	}
}

// ===========================================================================
// Thinking mode tests
// ===========================================================================

func TestLoop_ThinkingEnabled_DefaultTrue(t *testing.T) {
	// nil ThinkingEnabled in config should default to true.
	loop := NewLoop(LoopConfig{})
	if !loop.ThinkingEnabled() {
		t.Error("ThinkingEnabled should default to true when nil")
	}
}

func TestLoop_ThinkingEnabled_ExplicitTrue(t *testing.T) {
	enabled := true
	loop := NewLoop(LoopConfig{ThinkingEnabled: &enabled})
	if !loop.ThinkingEnabled() {
		t.Error("ThinkingEnabled should be true")
	}
}

func TestLoop_ThinkingEnabled_ExplicitFalse(t *testing.T) {
	disabled := false
	loop := NewLoop(LoopConfig{ThinkingEnabled: &disabled})
	if loop.ThinkingEnabled() {
		t.Error("ThinkingEnabled should be false")
	}
}

func TestLoop_SetThinkingEnabled(t *testing.T) {
	loop := NewLoop(LoopConfig{})

	loop.SetThinkingEnabled(false)
	if loop.ThinkingEnabled() {
		t.Error("after SetThinkingEnabled(false), should be false")
	}

	loop.SetThinkingEnabled(true)
	if !loop.ThinkingEnabled() {
		t.Error("after SetThinkingEnabled(true), should be true")
	}
}

// clearThinkingEnv ensures no env vars interfere with thinking config tests.
func clearThinkingEnv(t *testing.T) {
	t.Helper()
	t.Setenv("MAX_THINKING_TOKENS", "")
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "")
}

func TestLoop_BuildThinkingConfig_AdaptiveForOpus46(t *testing.T) {
	clearThinkingEnv(t)
	client := api.NewClient(nil, api.WithModel("claude-opus-4-6"))
	loop := NewLoop(LoopConfig{Client: client})

	cfg := loop.buildThinkingConfig()
	if cfg == nil {
		t.Fatal("expected non-nil thinking config for Opus 4.6")
	}
	if cfg.Type != "adaptive" {
		t.Errorf("Type = %q, want 'adaptive'", cfg.Type)
	}
	if cfg.BudgetTokens != 0 {
		t.Errorf("BudgetTokens = %d, want 0 (not used for adaptive)", cfg.BudgetTokens)
	}
}

func TestLoop_BuildThinkingConfig_AdaptiveForSonnet46(t *testing.T) {
	clearThinkingEnv(t)
	client := api.NewClient(nil, api.WithModel("claude-sonnet-4-6"))
	loop := NewLoop(LoopConfig{Client: client})

	cfg := loop.buildThinkingConfig()
	if cfg == nil {
		t.Fatal("expected non-nil thinking config for Sonnet 4.6")
	}
	if cfg.Type != "adaptive" {
		t.Errorf("Type = %q, want 'adaptive'", cfg.Type)
	}
}

func TestLoop_BuildThinkingConfig_BudgetForOlderModels(t *testing.T) {
	clearThinkingEnv(t)
	// claude-sonnet-4-20250514 supports thinking but not adaptive.
	client := api.NewClient(nil, api.WithModel("claude-sonnet-4-20250514"))
	loop := NewLoop(LoopConfig{Client: client})

	cfg := loop.buildThinkingConfig()
	if cfg == nil {
		t.Fatal("expected non-nil thinking config for Sonnet 4")
	}
	if cfg.Type != "enabled" {
		t.Errorf("Type = %q, want 'enabled'", cfg.Type)
	}
	if cfg.BudgetTokens != api.DefaultMaxTokens-1 {
		t.Errorf("BudgetTokens = %d, want %d", cfg.BudgetTokens, api.DefaultMaxTokens-1)
	}
}

func TestLoop_BuildThinkingConfig_NilForUnsupportedModels(t *testing.T) {
	clearThinkingEnv(t)
	// Haiku doesn't support thinking.
	client := api.NewClient(nil, api.WithModel("claude-haiku-4-5-20251001"))
	loop := NewLoop(LoopConfig{Client: client})

	cfg := loop.buildThinkingConfig()
	if cfg != nil {
		t.Errorf("expected nil thinking config for Haiku, got %+v", cfg)
	}
}

func TestLoop_BuildThinkingConfig_DisabledBySetting(t *testing.T) {
	clearThinkingEnv(t)
	client := api.NewClient(nil, api.WithModel("claude-opus-4-6"))
	disabled := false
	loop := NewLoop(LoopConfig{Client: client, ThinkingEnabled: &disabled})

	cfg := loop.buildThinkingConfig()
	if cfg != nil {
		t.Errorf("expected nil when thinking disabled, got %+v", cfg)
	}
}

func TestLoop_BuildThinkingConfig_DisableEnv(t *testing.T) {
	client := api.NewClient(nil, api.WithModel("claude-opus-4-6"))
	loop := NewLoop(LoopConfig{Client: client})

	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "1")

	cfg := loop.buildThinkingConfig()
	if cfg != nil {
		t.Errorf("expected nil with CLAUDE_CODE_DISABLE_THINKING=1, got %+v", cfg)
	}
}

func TestLoop_BuildThinkingConfig_MaxThinkingTokensEnv(t *testing.T) {
	client := api.NewClient(nil, api.WithModel("claude-opus-4-6"))
	loop := NewLoop(LoopConfig{Client: client})

	t.Setenv("MAX_THINKING_TOKENS", "5000")

	cfg := loop.buildThinkingConfig()
	if cfg == nil {
		t.Fatal("expected non-nil with MAX_THINKING_TOKENS=5000")
	}
	if cfg.Type != "enabled" {
		t.Errorf("Type = %q, want 'enabled'", cfg.Type)
	}
	if cfg.BudgetTokens != 5000 {
		t.Errorf("BudgetTokens = %d, want 5000", cfg.BudgetTokens)
	}
}

func TestLoop_BuildThinkingConfig_MaxThinkingTokensZero(t *testing.T) {
	client := api.NewClient(nil, api.WithModel("claude-opus-4-6"))
	loop := NewLoop(LoopConfig{Client: client})

	t.Setenv("MAX_THINKING_TOKENS", "0")

	cfg := loop.buildThinkingConfig()
	if cfg != nil {
		t.Errorf("expected nil with MAX_THINKING_TOKENS=0, got %+v", cfg)
	}
}

func TestEnvBoolTrue(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"yes", true},
		{"TRUE", true},
		{"YES", true},
		{"0", false},
		{"false", false},
		{"no", false},
		{"", false},
		{"maybe", false},
	}

	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			got := envBoolTrue(tt.val)
			if got != tt.want {
				t.Errorf("envBoolTrue(%q) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

// Verify that CLAUDE_CODE_DISABLE_THINKING is not read from a stale cache.
func TestLoop_BuildThinkingConfig_EnvNotCached(t *testing.T) {
	clearThinkingEnv(t)
	client := api.NewClient(nil, api.WithModel("claude-opus-4-6"))
	loop := NewLoop(LoopConfig{Client: client})

	// First call: no env set, should return adaptive.
	cfg := loop.buildThinkingConfig()
	if cfg == nil || cfg.Type != "adaptive" {
		t.Fatalf("expected adaptive, got %+v", cfg)
	}

	// Set env and call again: should return nil.
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "1")
	cfg = loop.buildThinkingConfig()
	if cfg != nil {
		t.Errorf("expected nil after setting env, got %+v", cfg)
	}
}
