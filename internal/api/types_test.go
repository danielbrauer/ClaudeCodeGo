package api

import (
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
