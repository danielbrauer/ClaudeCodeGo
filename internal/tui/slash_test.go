package tui

import (
	"strings"
	"testing"
)

func TestSlashRegistry_ClearCommandRegistered(t *testing.T) {
	r := newSlashRegistry()

	for _, name := range []string{"clear", "reset", "new"} {
		cmd, ok := r.lookup(name)
		if !ok {
			t.Errorf("/%s command not registered", name)
			continue
		}
		if cmd.Execute != nil {
			t.Errorf("/%s should have nil Execute (handled specially in handleSubmit)", name)
		}
		if cmd.Description == "" {
			t.Errorf("/%s has empty description", name)
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

func TestSlashRegistry_ClearCompletion(t *testing.T) {
	r := newSlashRegistry()

	// "cl" should complete to "clear".
	matches := r.complete("cl")
	found := false
	for _, m := range matches {
		if m == "clear" {
			found = true
		}
	}
	if !found {
		t.Errorf("complete(\"cl\") = %v, want to contain \"clear\"", matches)
	}

	// "re" should complete to "reset".
	matches = r.complete("re")
	found = false
	for _, m := range matches {
		if m == "reset" {
			found = true
		}
	}
	if !found {
		t.Errorf("complete(\"re\") = %v, want to contain \"reset\"", matches)
	}

	// "ne" should complete to "new".
	matches = r.complete("ne")
	found = false
	for _, m := range matches {
		if m == "new" {
			found = true
		}
	}
	if !found {
		t.Errorf("complete(\"ne\") = %v, want to contain \"new\"", matches)
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
