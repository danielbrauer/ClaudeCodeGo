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
	}
	for _, name := range expected {
		if _, ok := r.lookup(name); !ok {
			t.Errorf("expected built-in command %q to be registered", name)
		}
	}
}

func TestSlashRegistry_MemoryAndInitAreNilExecute(t *testing.T) {
	r := newSlashRegistry()

	// memory and init are handled specially in handleSubmit, so Execute is nil.
	for _, name := range []string{"memory", "init"} {
		cmd, ok := r.lookup(name)
		if !ok {
			t.Fatalf("command %q not found", name)
		}
		if cmd.Execute != nil {
			t.Errorf("command %q should have nil Execute (handled specially)", name)
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
		{"co", []string{"compact", "context", "cost"}},
		{"qu", []string{"quit"}},
		{"init", []string{"init"}},
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

	// Execute should return the skill prefix + content.
	output := cmd.Execute(nil)
	if !strings.HasPrefix(output, skillCommandPrefix) {
		t.Errorf("skill Execute should return sentinel prefix, got %q", output)
	}
	content := strings.TrimPrefix(output, skillCommandPrefix)
	if content != "Please commit with a good message." {
		t.Errorf("skill content = %q, want %q", content, "Please commit with a good message.")
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
