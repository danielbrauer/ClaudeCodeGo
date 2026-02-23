package tui

import (
	"strings"
	"testing"
)

func TestInitPrompt_ContainsKeyPhrases(t *testing.T) {
	checks := []string{
		"analyze this codebase",
		"create a CLAUDE.md file",
		"how to build, lint, and run tests",
		"High-level code architecture",
		"If there's already a CLAUDE.md, suggest improvements",
		"# CLAUDE.md",
		"This file provides guidance to Claude Code",
	}
	for _, phrase := range checks {
		if !strings.Contains(initPrompt, phrase) {
			t.Errorf("initPrompt should contain %q", phrase)
		}
	}
}

func TestInitPrompt_NotEmpty(t *testing.T) {
	if strings.TrimSpace(initPrompt) == "" {
		t.Error("initPrompt should not be empty")
	}
}
