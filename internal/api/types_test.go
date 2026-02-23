package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveModelAlias(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Known aliases resolve to full model IDs.
		{"opus", ModelClaude46Opus},
		{"sonnet", ModelClaude46Sonnet},
		{"haiku", ModelClaude45Haiku},
		// Full model IDs pass through unchanged.
		{ModelClaude46Opus, ModelClaude46Opus},
		{ModelClaude46Sonnet, ModelClaude46Sonnet},
		{ModelClaude45Haiku, ModelClaude45Haiku},
		// Unknown strings pass through unchanged.
		{"claude-unknown-model", "claude-unknown-model"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveModelAlias(tt.input)
			if got != tt.want {
				t.Errorf("ResolveModelAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestModelDisplayName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Full model IDs return display names.
		{ModelClaude46Opus, "Opus 4.6"},
		{ModelClaude46Sonnet, "Sonnet 4.6"},
		{ModelClaude45Haiku, "Haiku 4.5"},
		// Aliases also return display names.
		{"opus", "Opus 4.6"},
		{"sonnet", "Sonnet 4.6"},
		{"haiku", "Haiku 4.5"},
		// Unknown models return the input string as-is.
		{"claude-unknown", "claude-unknown"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ModelDisplayName(tt.input)
			if got != tt.want {
				t.Errorf("ModelDisplayName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAvailableModels(t *testing.T) {
	if len(AvailableModels) == 0 {
		t.Fatal("AvailableModels is empty")
	}

	// Every model should have non-empty fields.
	for i, opt := range AvailableModels {
		if opt.Alias == "" {
			t.Errorf("AvailableModels[%d].Alias is empty", i)
		}
		if opt.ID == "" {
			t.Errorf("AvailableModels[%d].ID is empty", i)
		}
		if opt.DisplayName == "" {
			t.Errorf("AvailableModels[%d].DisplayName is empty", i)
		}
		if opt.Description == "" {
			t.Errorf("AvailableModels[%d].Description is empty", i)
		}
	}

	// Every alias in AvailableModels should resolve to the corresponding ID.
	for _, opt := range AvailableModels {
		resolved := ResolveModelAlias(opt.Alias)
		if resolved != opt.ID {
			t.Errorf("ResolveModelAlias(%q) = %q, want %q (from AvailableModels)", opt.Alias, resolved, opt.ID)
		}
	}

	// Every ID in AvailableModels should produce the correct display name.
	for _, opt := range AvailableModels {
		display := ModelDisplayName(opt.ID)
		if display != opt.DisplayName {
			t.Errorf("ModelDisplayName(%q) = %q, want %q", opt.ID, display, opt.DisplayName)
		}
	}
}

func TestModelAliases_Complete(t *testing.T) {
	// Every alias in ModelAliases should appear in AvailableModels.
	for alias, id := range ModelAliases {
		found := false
		for _, opt := range AvailableModels {
			if opt.Alias == alias && opt.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ModelAliases[%q] = %q has no matching entry in AvailableModels", alias, id)
		}
	}
}

func TestModelConstants(t *testing.T) {
	// Verify model IDs match expected patterns.
	if ModelClaude46Opus != "claude-opus-4-6" {
		t.Errorf("ModelClaude46Opus = %q, want %q", ModelClaude46Opus, "claude-opus-4-6")
	}
	if ModelClaude46Sonnet != "claude-sonnet-4-6" {
		t.Errorf("ModelClaude46Sonnet = %q, want %q", ModelClaude46Sonnet, "claude-sonnet-4-6")
	}
	if ModelClaude45Haiku != "claude-haiku-4-5-20251001" {
		t.Errorf("ModelClaude45Haiku = %q, want %q", ModelClaude45Haiku, "claude-haiku-4-5-20251001")
	}
}

// ===========================================================================
// SupportsThinking
// ===========================================================================

func TestSupportsThinking(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-opus-4-6", true},
		{"claude-opus-4-6-20260101", true},
		{"claude-sonnet-4-6", true},
		{"claude-sonnet-4-6-20260101", true},
		{"claude-sonnet-4-20250514", true},
		{"claude-opus-4-20250514", true},
		{"CLAUDE-OPUS-4-6", true},         // case insensitive
		{"claude-haiku-4-5-20251001", false}, // Haiku doesn't have sonnet-4 or opus-4
		{"claude-3-5-sonnet-20241022", false},
		{"gpt-4", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := SupportsThinking(tt.model)
			if got != tt.want {
				t.Errorf("SupportsThinking(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

// ===========================================================================
// SupportsAdaptiveThinking
// ===========================================================================

func TestSupportsAdaptiveThinking(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-opus-4-6", true},
		{"claude-opus-4-6-20260101", true},
		{"claude-sonnet-4-6", true},
		{"claude-sonnet-4-6-20260101", true},
		{"CLAUDE-OPUS-4-6", true},            // case insensitive
		{"claude-sonnet-4-20250514", false},   // Sonnet 4 (not 4.6)
		{"claude-opus-4-20250514", false},     // Opus 4 (not 4.6)
		{"claude-haiku-4-5-20251001", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := SupportsAdaptiveThinking(tt.model)
			if got != tt.want {
				t.Errorf("SupportsAdaptiveThinking(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

// ===========================================================================
// ThinkingConfig types
// ===========================================================================

func TestThinkingAdaptive(t *testing.T) {
	cfg := ThinkingAdaptive()
	if cfg.Type != "adaptive" {
		t.Errorf("Type = %q, want 'adaptive'", cfg.Type)
	}
	if cfg.BudgetTokens != 0 {
		t.Errorf("BudgetTokens = %d, want 0", cfg.BudgetTokens)
	}
}

func TestThinkingEnabled(t *testing.T) {
	cfg := ThinkingEnabled(10000)
	if cfg.Type != "enabled" {
		t.Errorf("Type = %q, want 'enabled'", cfg.Type)
	}
	if cfg.BudgetTokens != 10000 {
		t.Errorf("BudgetTokens = %d, want 10000", cfg.BudgetTokens)
	}
}

func TestThinkingDisabled(t *testing.T) {
	cfg := ThinkingDisabled()
	if cfg.Type != "disabled" {
		t.Errorf("Type = %q, want 'disabled'", cfg.Type)
	}
}

func TestThinkingConfigSerialization(t *testing.T) {
	// Adaptive: should serialize without budget_tokens.
	req := &CreateMessageRequest{
		Model:    "claude-opus-4-6",
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
		Thinking: ThinkingAdaptive(),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"thinking":{"type":"adaptive"}`) {
		t.Errorf("adaptive thinking serialization wrong: %s", data)
	}

	// Enabled with budget.
	req2 := &CreateMessageRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
		Thinking: ThinkingEnabled(8000),
	}
	data2, _ := json.Marshal(req2)
	if !strings.Contains(string(data2), `"thinking":{"type":"enabled","budget_tokens":8000}`) {
		t.Errorf("enabled thinking serialization wrong: %s", data2)
	}

	// nil thinking should be omitted.
	req3 := &CreateMessageRequest{
		Model:    "claude-haiku-4-5-20251001",
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}
	data3, _ := json.Marshal(req3)
	if strings.Contains(string(data3), "thinking") {
		t.Errorf("nil thinking should be omitted, got: %s", data3)
	}
}
