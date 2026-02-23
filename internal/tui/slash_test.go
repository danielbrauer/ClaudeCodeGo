package tui

import (
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/skills"
)

func TestNewSlashRegistry_BuiltinCommands(t *testing.T) {
	r := newSlashRegistry()

	expected := []string{
		"help", "model", "version", "cost", "context",
		"mcp", "memory", "init", "compact", "quit", "exit",
		"clear", "reset", "new",
	}
	for _, name := range expected {
		if _, ok := r.lookup(name); !ok {
			t.Errorf("expected built-in command %q to be registered", name)
		}
	}
}

func TestSlashRegistry_ClearCommandRegistered(t *testing.T) {
	r := newSlashRegistry()

	for _, name := range []string{"clear", "reset", "new"} {
		cmd, ok := r.lookup(name)
		if !ok {
			t.Errorf("/%s command not registered", name)
			continue
		}
		if cmd.Execute == nil {
			t.Errorf("/%s should have non-nil Execute", name)
		}
		if cmd.Description == "" {
			t.Errorf("/%s has empty description", name)
		}
	}
}

func TestSlashRegistry_AllCommandsHaveExecute(t *testing.T) {
	r := newSlashRegistry()

	// All registered commands should have a non-nil Execute handler.
	for _, name := range r.names {
		cmd := r.commands[name]
		if cmd.Execute == nil {
			t.Errorf("command %q has nil Execute; all commands must be self-contained", name)
		}
	}
}

func TestSlashRegistry_Lookup(t *testing.T) {
	r := newSlashRegistry()

	if _, ok := r.lookup("help"); !ok {
		t.Error("lookup(\"help\") should find the command")
	}
	if _, ok := r.lookup("nonexistent"); ok {
		t.Error("lookup(\"nonexistent\") should not find a command")
	}
}

func TestSlashRegistry_Complete(t *testing.T) {
	r := newSlashRegistry()

	tests := []struct {
		prefix string
		want   []string
	}{
		{"he", []string{"help"}},
		{"m", []string{"mcp", "memory", "model"}},
		{"co", []string{"compact", "config", "context", "continue", "cost"}},
		{"qu", []string{"quit"}},
		{"init", []string{"init"}},
		{"cl", []string{"clear"}},
		{"re", []string{"reset", "resume", "review"}},
		{"di", []string{"diff"}},
		{"ne", []string{"new"}},
		{"xyz", nil},
	}

	for _, tt := range tests {
		got := r.complete(tt.prefix)
		if len(got) != len(tt.want) {
			t.Errorf("complete(%q) = %v, want %v", tt.prefix, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("complete(%q)[%d] = %q, want %q", tt.prefix, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSlashRegistry_ClearAliasesHiddenFromHelp(t *testing.T) {
	r := newSlashRegistry()
	help := r.helpText()

	// /clear should appear in help.
	if !strings.Contains(help, "/clear") {
		t.Error("help text should contain /clear")
	}

	// Aliases /reset and /new should not appear in help.
	for _, alias := range []string{"/reset", "/new"} {
		// Check that the alias doesn't appear as a command entry.
		// It could appear as part of description text, so check the command column.
		lines := strings.Split(help, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, alias+" ") || strings.HasPrefix(trimmed, alias+"\t") {
				t.Errorf("help text should not list %s as a command, found: %q", alias, line)
			}
		}
	}
}

func TestSlashRegistry_ClearDescription(t *testing.T) {
	r := newSlashRegistry()

	// All three should have the same description.
	clear, _ := r.lookup("clear")
	reset, _ := r.lookup("reset")
	new_, _ := r.lookup("new")

	if clear.Description != reset.Description {
		t.Errorf("/clear description %q != /reset description %q", clear.Description, reset.Description)
	}
	if clear.Description != new_.Description {
		t.Errorf("/clear description %q != /new description %q", clear.Description, new_.Description)
	}
}

func TestSlashRegistry_HelpText(t *testing.T) {
	r := newSlashRegistry()
	help := r.helpText()

	// Should include memory and init.
	if !strings.Contains(help, "/memory") {
		t.Error("helpText should include /memory")
	}
	if !strings.Contains(help, "/init") {
		t.Error("helpText should include /init")
	}

	// Should not include "exit" (it's hidden since "quit" is listed).
	for _, line := range strings.Split(help, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "/exit") {
			t.Error("helpText should not show /exit (hidden alias for /quit)")
		}
	}
}

func TestSlashRegistry_RegisterSkills(t *testing.T) {
	r := newSlashRegistry()

	loadedSkills := []skills.Skill{
		{
			Name:        "commit",
			Description: "Create a git commit",
			Trigger:     "/commit",
			Content:     "Please commit with a good message.",
		},
		{
			Name:        "no-trigger",
			Description: "Skill without trigger",
			Trigger:     "",
			Content:     "ignored",
		},
	}

	r.registerSkills(loadedSkills)

	// /commit should be registered.
	cmd, ok := r.lookup("commit")
	if !ok {
		t.Fatal("expected /commit to be registered")
	}
	if cmd.Execute == nil {
		t.Fatal("skill command should have non-nil Execute")
	}
	if !cmd.IsSkill {
		t.Error("skill command should have IsSkill=true")
	}

	// no-trigger skill should not be registered.
	if _, ok := r.lookup("no-trigger"); ok {
		t.Error("skill without trigger should not be registered")
	}
}

func TestSlashRegistry_NamesSorted(t *testing.T) {
	r := newSlashRegistry()

	for i := 1; i < len(r.names); i++ {
		if r.names[i-1] > r.names[i] {
			t.Errorf("names not sorted: %q > %q", r.names[i-1], r.names[i])
		}
	}
}
