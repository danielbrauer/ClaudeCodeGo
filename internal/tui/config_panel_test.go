package tui

import (
	"testing"

	"github.com/anthropics/claude-code-go/internal/config"
)

func TestNewConfigPanel(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	if cp == nil {
		t.Fatal("expected non-nil configPanel")
	}
	if cp.settings != s {
		t.Error("configPanel.settings should reference the provided settings")
	}
	if cp.initial == nil {
		t.Error("configPanel.initial should be set for change tracking")
	}
	if len(cp.items) == 0 {
		t.Error("configPanel should have items")
	}
	if len(cp.filtered) != len(cp.items) {
		t.Errorf("filtered len = %d, want %d", len(cp.filtered), len(cp.items))
	}
	if cp.cursor != 0 {
		t.Errorf("cursor = %d, want 0", cp.cursor)
	}
}

func TestConfigPanel_GetValue_BoolDefaults(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	tests := []struct {
		id   string
		want string
	}{
		{"autoCompactEnabled", "true"},          // default true
		{"alwaysThinkingEnabled", "true"},        // default true
		{"fastMode", "false"},                    // default false
		{"verbose", "false"},                     // default false
		{"respectGitignore", "true"},             // default true
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			item := findItem(cp, tt.id)
			if item == nil {
				t.Fatalf("item %q not found", tt.id)
			}
			got := cp.getValue(*item)
			if got != tt.want {
				t.Errorf("getValue(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestConfigPanel_GetValue_EnumDefaults(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	tests := []struct {
		id   string
		want string
	}{
		{"editorMode", "normal"},
		{"diffTool", "auto"},
		{"theme", "dark"},
		{"notifChannel", "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			item := findItem(cp, tt.id)
			if item == nil {
				t.Fatalf("item %q not found", tt.id)
			}
			got := cp.getValue(*item)
			if got != tt.want {
				t.Errorf("getValue(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestConfigPanel_GetValue_ExplicitValues(t *testing.T) {
	s := &config.Settings{
		AutoCompactEnabled: config.BoolPtr(false),
		FastMode:           config.BoolPtr(true),
		EditorMode:         "vim",
		Theme:              "light",
	}
	cp := newConfigPanel(s)

	tests := []struct {
		id   string
		want string
	}{
		{"autoCompactEnabled", "false"},
		{"fastMode", "true"},
		{"editorMode", "vim"},
		{"theme", "light"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			item := findItem(cp, tt.id)
			if item == nil {
				t.Fatalf("item %q not found", tt.id)
			}
			got := cp.getValue(*item)
			if got != tt.want {
				t.Errorf("getValue(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestConfigPanel_ToggleBool(t *testing.T) {
	// Use a temp HOME to avoid writing to real home.
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	// autoCompactEnabled defaults to true; toggling should make it false.
	cp.toggleBool("autoCompactEnabled", s)
	if s.AutoCompactEnabled == nil || *s.AutoCompactEnabled != false {
		t.Errorf("after toggle, AutoCompactEnabled = %v, want false", s.AutoCompactEnabled)
	}

	// Toggle again: should become true.
	cp.toggleBool("autoCompactEnabled", s)
	if s.AutoCompactEnabled == nil || *s.AutoCompactEnabled != true {
		t.Errorf("after double toggle, AutoCompactEnabled = %v, want true", s.AutoCompactEnabled)
	}
}

func TestConfigPanel_ToggleBool_ThinkingMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	// alwaysThinkingEnabled defaults to true; toggling should make it false.
	cp.toggleBool("alwaysThinkingEnabled", s)
	if s.ThinkingEnabled == nil || *s.ThinkingEnabled != false {
		t.Errorf("after toggle, ThinkingEnabled = %v, want false", s.ThinkingEnabled)
	}

	// Toggle again: should become true.
	cp.toggleBool("alwaysThinkingEnabled", s)
	if s.ThinkingEnabled == nil || *s.ThinkingEnabled != true {
		t.Errorf("after double toggle, ThinkingEnabled = %v, want true", s.ThinkingEnabled)
	}
}

func TestConfigPanel_ToggleBool_FalseDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	// fastMode defaults to false; toggling should make it true.
	cp.toggleBool("fastMode", s)
	if s.FastMode == nil || *s.FastMode != true {
		t.Errorf("after toggle, FastMode = %v, want true", s.FastMode)
	}
}

func TestConfigPanel_CycleEnum(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	// editorMode defaults to "normal" (first option); cycle should go to "vim".
	cp.cycleEnum("editorMode", []string{"normal", "vim"}, s)
	if s.EditorMode != "vim" {
		t.Errorf("after first cycle, EditorMode = %q, want %q", s.EditorMode, "vim")
	}

	// Cycle again: should wrap back to "normal".
	cp.cycleEnum("editorMode", []string{"normal", "vim"}, s)
	if s.EditorMode != "normal" {
		t.Errorf("after second cycle, EditorMode = %q, want %q", s.EditorMode, "normal")
	}
}

func TestConfigPanel_CycleEnum_MultipleOptions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	opts := []string{"dark", "light", "dark-daltonized", "light-daltonized"}

	// Cycle through all theme options.
	expected := []string{"light", "dark-daltonized", "light-daltonized", "dark"}
	for i, want := range expected {
		cp.cycleEnum("theme", opts, s)
		if s.Theme != want {
			t.Errorf("cycle %d: Theme = %q, want %q", i+1, s.Theme, want)
		}
	}
}

func TestConfigPanel_ToggleOrCycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	// cursor is at 0 which is "autoCompactEnabled" (bool).
	cp.cursor = 0
	cp.toggleOrCycle()
	if s.AutoCompactEnabled == nil || *s.AutoCompactEnabled != false {
		t.Errorf("toggleOrCycle on bool: AutoCompactEnabled = %v, want false", s.AutoCompactEnabled)
	}

	// Move cursor to editorMode (index 5 = "Editor mode" enum).
	cp.cursor = 5
	cp.toggleOrCycle()
	if s.EditorMode != "vim" {
		t.Errorf("toggleOrCycle on enum: EditorMode = %q, want %q", s.EditorMode, "vim")
	}
}

func TestConfigPanel_ToggleOrCycle_EmptyFilter(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)
	cp.filtered = cp.filtered[:0] // empty filter

	// Should not panic.
	cp.toggleOrCycle()
}

func TestConfigPanel_ApplyFilter(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	// Search for "mode" should match "Thinking mode", "Fast mode", "Editor mode".
	cp.searchQuery = "mode"
	cp.applyFilter()

	if len(cp.filtered) != 3 {
		t.Errorf("filtered for 'mode': len = %d, want 3", len(cp.filtered))
	}

	// Verify filtered items are the right ones.
	for _, idx := range cp.filtered {
		item := cp.items[idx]
		if item.id != "alwaysThinkingEnabled" && item.id != "fastMode" && item.id != "editorMode" {
			t.Errorf("unexpected filtered item: %q", item.id)
		}
	}
}

func TestConfigPanel_ApplyFilter_NoMatch(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	cp.searchQuery = "zzzznoexist"
	cp.applyFilter()

	if len(cp.filtered) != 0 {
		t.Errorf("filtered for 'zzzznoexist': len = %d, want 0", len(cp.filtered))
	}
	// Cursor should be clamped to 0.
	if cp.cursor != 0 {
		t.Errorf("cursor = %d, want 0", cp.cursor)
	}
}

func TestConfigPanel_ApplyFilter_CaseInsensitive(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	cp.searchQuery = "COMPACT"
	cp.applyFilter()

	if len(cp.filtered) != 1 {
		t.Errorf("filtered for 'COMPACT': len = %d, want 1", len(cp.filtered))
	}
	if len(cp.filtered) > 0 {
		item := cp.items[cp.filtered[0]]
		if item.id != "autoCompactEnabled" {
			t.Errorf("expected autoCompactEnabled, got %q", item.id)
		}
	}
}

func TestConfigPanel_ResetFilter(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	cp.searchQuery = "mode"
	cp.applyFilter()
	if len(cp.filtered) == len(cp.items) {
		t.Fatal("filter should have reduced the list")
	}

	cp.resetFilter()
	if len(cp.filtered) != len(cp.items) {
		t.Errorf("after resetFilter: len = %d, want %d", len(cp.filtered), len(cp.items))
	}
}

func TestConfigPanel_ApplyFilter_EmptyQuery(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	cp.searchQuery = ""
	cp.applyFilter()

	if len(cp.filtered) != len(cp.items) {
		t.Errorf("empty query should show all: len = %d, want %d", len(cp.filtered), len(cp.items))
	}
}

func TestConfigPanel_BuildChangeSummary_NoChanges(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	changes := cp.buildChangeSummary()
	if len(changes) != 0 {
		t.Errorf("no changes expected, got %v", changes)
	}
}

func TestConfigPanel_BuildChangeSummary_BoolChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	// Toggle fastMode (default false → true).
	cp.toggleBool("fastMode", s)

	changes := cp.buildChangeSummary()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	if changes[0] != "Enabled fast mode" {
		t.Errorf("change = %q, want %q", changes[0], "Enabled fast mode")
	}
}

func TestConfigPanel_BuildChangeSummary_BoolDisable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	// Toggle autoCompactEnabled (default true → false).
	cp.toggleBool("autoCompactEnabled", s)

	changes := cp.buildChangeSummary()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	if changes[0] != "Disabled auto-compact" {
		t.Errorf("change = %q, want %q", changes[0], "Disabled auto-compact")
	}
}

func TestConfigPanel_BuildChangeSummary_EnumChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	// Cycle theme (default "dark" → "light").
	cp.cycleEnum("theme", []string{"dark", "light", "dark-daltonized", "light-daltonized"}, s)

	changes := cp.buildChangeSummary()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	if changes[0] != "Set theme to light" {
		t.Errorf("change = %q, want %q", changes[0], "Set theme to light")
	}
}

func TestConfigPanel_BuildChangeSummary_MultipleChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s := &config.Settings{}
	cp := newConfigPanel(s)

	cp.toggleBool("verbose", s)
	cp.cycleEnum("editorMode", []string{"normal", "vim"}, s)

	changes := cp.buildChangeSummary()
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %v", len(changes), changes)
	}
}

func TestConfigPanel_SnapshotSettings(t *testing.T) {
	s := &config.Settings{
		AutoCompactEnabled: config.BoolPtr(true),
		FastMode:           config.BoolPtr(false),
		EditorMode:         "vim",
	}

	snap := snapshotSettings(s)

	// Modify original; snapshot should be unaffected.
	*s.AutoCompactEnabled = false
	s.EditorMode = "normal"

	if snap.AutoCompactEnabled == nil || *snap.AutoCompactEnabled != true {
		t.Errorf("snapshot AutoCompactEnabled changed: %v", snap.AutoCompactEnabled)
	}
	if snap.EditorMode != "vim" {
		t.Errorf("snapshot EditorMode changed: %q", snap.EditorMode)
	}
}

func TestConfigPanel_ViewportHeight(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	// Normal terminal: plenty of room.
	h := cp.viewportHeight(40)
	if h != 25 { // 40 - 15
		t.Errorf("viewportHeight(40) = %d, want 25", h)
	}

	// Tiny terminal: clamp to minimum.
	h = cp.viewportHeight(10)
	if h != 5 {
		t.Errorf("viewportHeight(10) = %d, want 5", h)
	}
}

func TestConfigPanel_ApplyFilter_CursorClamp(t *testing.T) {
	s := &config.Settings{}
	cp := newConfigPanel(s)

	// Set cursor to the last item.
	cp.cursor = len(cp.items) - 1

	// Apply a filter that matches only 1 item.
	cp.searchQuery = "compact"
	cp.applyFilter()

	if cp.cursor >= len(cp.filtered) {
		t.Errorf("cursor should be clamped: cursor = %d, filtered len = %d", cp.cursor, len(cp.filtered))
	}
}

// findItem finds a configSetting by id in the panel.
func findItem(cp *configPanel, id string) *configSetting {
	for i := range cp.items {
		if cp.items[i].id == id {
			return &cp.items[i]
		}
	}
	return nil
}
