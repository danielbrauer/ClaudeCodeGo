package tui

import (
	"testing"

	"github.com/anthropics/claude-code-go/internal/config"
)

func TestE2E_ConfigCommand_OpensPanel(t *testing.T) {
	settings := &config.Settings{}
	m, _ := testModel(t, withSettings(settings))

	result, _ := submitCommand(m, "/config")

	if result.mode != modeConfig {
		t.Errorf("mode = %d, want modeConfig (%d)", result.mode, modeConfig)
	}
	if result.configPanel == nil {
		t.Error("configPanel should be non-nil after /config")
	}
}

func TestE2E_SettingsCommand_IsAliasForConfig(t *testing.T) {
	settings := &config.Settings{}
	m, _ := testModel(t, withSettings(settings))

	result, _ := submitCommand(m, "/settings")

	if result.mode != modeConfig {
		t.Errorf("mode = %d, want modeConfig (%d)", result.mode, modeConfig)
	}
	if result.configPanel == nil {
		t.Error("configPanel should be non-nil after /settings")
	}
}

func TestE2E_ConfigCommand_NoSettings(t *testing.T) {
	m, _ := testModel(t) // settings is nil

	result, _ := submitCommand(m, "/config")

	// Should remain in input mode (error printed).
	if result.mode != modeInput {
		t.Errorf("mode = %d, want modeInput (no settings)", result.mode)
	}
	if result.configPanel != nil {
		t.Error("configPanel should be nil when no settings available")
	}
}

func TestE2E_ConfigCommand_BlursInput(t *testing.T) {
	settings := &config.Settings{}
	m, _ := testModel(t, withSettings(settings))

	result, _ := submitCommand(m, "/config")

	if result.textInput.Focused() {
		t.Error("text input should be blurred when config panel is open")
	}
}

func TestE2E_ConfigCommand_PanelHasItems(t *testing.T) {
	settings := &config.Settings{}
	m, _ := testModel(t, withSettings(settings))

	result, _ := submitCommand(m, "/config")

	if result.configPanel == nil {
		t.Fatal("configPanel is nil")
	}
	if len(result.configPanel.items) == 0 {
		t.Error("config panel should have settings items")
	}

	// Check that key settings are present.
	ids := make(map[string]bool)
	for _, item := range result.configPanel.items {
		ids[item.id] = true
	}
	for _, expected := range []string{"autoCompactEnabled", "fastMode", "theme", "editorMode"} {
		if !ids[expected] {
			t.Errorf("config panel should include setting %q", expected)
		}
	}
}

func TestE2E_ConfigCommand_ReflectsCurrentSettings(t *testing.T) {
	settings := &config.Settings{
		FastMode:   config.BoolPtr(true),
		EditorMode: "vim",
		Theme:      "light",
	}
	m, _ := testModel(t, withSettings(settings))

	result, _ := submitCommand(m, "/config")

	cp := result.configPanel
	if cp == nil {
		t.Fatal("configPanel is nil")
	}

	// Check that the panel reflects the current settings values.
	for _, item := range cp.items {
		val := cp.getValue(item)
		switch item.id {
		case "fastMode":
			if val != "true" {
				t.Errorf("fastMode value = %q, want true", val)
			}
		case "editorMode":
			if val != "vim" {
				t.Errorf("editorMode value = %q, want vim", val)
			}
		case "theme":
			if val != "light" {
				t.Errorf("theme value = %q, want light", val)
			}
		}
	}
}
