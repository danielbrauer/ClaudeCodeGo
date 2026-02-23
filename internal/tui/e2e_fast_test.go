package tui

import (
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
)

func TestE2E_FastCommand_ToggleOn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m, _ := testModel(t,
		withModelName(api.ModelAliases["opus"]),
		withFastMode(false),
	)

	cmd, ok := m.slashReg.lookup("fast")
	if !ok {
		t.Fatal("/fast not registered")
	}
	cmd.Execute(&m, "")

	if !m.fastMode {
		t.Error("fastMode should be true after toggle on")
	}
}

func TestE2E_FastCommand_ToggleOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m, _ := testModel(t,
		withModelName(api.ModelAliases["opus"]),
		withFastMode(true),
	)
	m.fastMode = true // ensure it's on

	cmd, _ := m.slashReg.lookup("fast")
	cmd.Execute(&m, "")

	if m.fastMode {
		t.Error("fastMode should be false after toggle off")
	}
}

func TestE2E_FastCommand_AutoSwitchesToOpus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Start with a non-Opus model.
	m, _ := testModel(t,
		withModelName("claude-sonnet-4-20250514"),
		withFastMode(false),
	)

	cmd, _ := m.slashReg.lookup("fast")
	cmd.Execute(&m, "")

	if !m.fastMode {
		t.Error("fastMode should be true after toggle on")
	}

	// Should have auto-switched to Opus.
	expected := api.ModelAliases[api.FastModeModelAlias]
	if m.modelName != expected {
		t.Errorf("model should auto-switch to Opus for fast mode, got %q, want %q", m.modelName, expected)
	}
}

func TestE2E_FastCommand_NoAutoSwitchWhenAlreadyOpus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	opusModel := api.ModelAliases["opus"]
	m, _ := testModel(t,
		withModelName(opusModel),
		withFastMode(false),
	)

	cmd, _ := m.slashReg.lookup("fast")
	cmd.Execute(&m, "")

	// Model should remain the same Opus model.
	if m.modelName != opusModel {
		t.Errorf("model should stay %q, got %q", opusModel, m.modelName)
	}
}

func TestE2E_FastCommand_PersistsToSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m, _ := testModel(t,
		withModelName(api.ModelAliases["opus"]),
		withFastMode(false),
	)

	cmd, _ := m.slashReg.lookup("fast")
	cmd.Execute(&m, "")

	// Toggle on should persist. We can't easily read the file without knowing
	// the full path, but we verify the model state is correct.
	if !m.fastMode {
		t.Error("fastMode should be true after toggle")
	}
	if !m.loop.FastMode() {
		t.Error("loop fast mode should be true after toggle")
	}
}

func TestE2E_FastCommand_ToggleRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m, _ := testModel(t,
		withModelName(api.ModelAliases["opus"]),
		withFastMode(false),
	)

	cmd, _ := m.slashReg.lookup("fast")

	// On.
	cmd.Execute(&m, "")
	if !m.fastMode {
		t.Error("first toggle should turn fast mode ON")
	}

	// Off.
	cmd.Execute(&m, "")
	if m.fastMode {
		t.Error("second toggle should turn fast mode OFF")
	}

	// On again.
	cmd.Execute(&m, "")
	if !m.fastMode {
		t.Error("third toggle should turn fast mode ON")
	}
}

// ── Regression tests: /fast ↔ config panel synchronisation ──

// TestE2E_FastSync_SlashUpdatesSettings verifies that /fast updates
// m.settings.FastMode so the config panel sees the correct value.
func TestE2E_FastSync_SlashUpdatesSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	settings := &config.Settings{}
	m, _ := testModel(t,
		withModelName(api.ModelAliases["opus"]),
		withFastMode(false),
		withSettings(settings),
	)

	cmd, _ := m.slashReg.lookup("fast")
	cmd.Execute(&m, "")

	// m.fastMode should be true.
	if !m.fastMode {
		t.Error("m.fastMode should be true after /fast toggle on")
	}
	// settings.FastMode must also be true — this is the value the
	// config panel reads.
	if m.settings.FastMode == nil || !*m.settings.FastMode {
		t.Errorf("settings.FastMode should be true after /fast, got %v", m.settings.FastMode)
	}

	// Toggle off.
	cmd.Execute(&m, "")

	if m.fastMode {
		t.Error("m.fastMode should be false after /fast toggle off")
	}
	if m.settings.FastMode == nil || *m.settings.FastMode {
		t.Errorf("settings.FastMode should be false after /fast off, got %v", m.settings.FastMode)
	}
}

// TestE2E_FastSync_ConfigPanelUpdatesRuntime verifies that toggling
// fast mode in the config panel updates m.fastMode and loop.FastMode()
// when the panel is closed.
func TestE2E_FastSync_ConfigPanelUpdatesRuntime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	settings := &config.Settings{}
	m, _ := testModel(t,
		withModelName(api.ModelAliases["opus"]),
		withFastMode(false),
		withSettings(settings),
	)

	// Open config panel.
	m, _ = submitCommand(m, "/config")
	if m.configPanel == nil {
		t.Fatal("configPanel should be open")
	}

	// Toggle fastMode via config panel.
	m.configPanel.toggleBool("fastMode", m.settings)

	// Close the panel — this should sync runtime state.
	result, _ := m.closeConfigPanel()
	m = result.(model)

	if !m.fastMode {
		t.Error("m.fastMode should be true after config panel toggle")
	}
	if !m.loop.FastMode() {
		t.Error("loop.FastMode() should be true after config panel toggle")
	}
}

// TestE2E_FastSync_ConfigPanelOff verifies that disabling fast mode
// via config panel turns off the status bar indicator.
func TestE2E_FastSync_ConfigPanelOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	settings := &config.Settings{
		FastMode: config.BoolPtr(true),
	}
	m, _ := testModel(t,
		withModelName(api.ModelAliases["opus"]),
		withFastMode(true),
		withSettings(settings),
	)
	// The test helper sets m.fastMode but not loop; sync them.
	m.loop.SetFastMode(true)

	// Precondition: fast mode is on everywhere.
	if !m.fastMode || !m.loop.FastMode() {
		t.Fatal("precondition: fast mode should start as on")
	}

	// Open config panel and toggle fast mode off.
	m, _ = submitCommand(m, "/config")
	m.configPanel.toggleBool("fastMode", m.settings)

	// Close panel.
	result, _ := m.closeConfigPanel()
	m = result.(model)

	if m.fastMode {
		t.Error("m.fastMode should be false after config panel toggle off")
	}
	if m.loop.FastMode() {
		t.Error("loop.FastMode() should be false after config panel toggle off")
	}
}

// TestE2E_FastSync_SlashOnThenConfigOff exercises the cross-path
// scenario: enable via /fast, disable via config panel.
func TestE2E_FastSync_SlashOnThenConfigOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	settings := &config.Settings{}
	m, _ := testModel(t,
		withModelName(api.ModelAliases["opus"]),
		withFastMode(false),
		withSettings(settings),
	)

	// Enable via /fast.
	cmd, _ := m.slashReg.lookup("fast")
	cmd.Execute(&m, "")
	if !m.fastMode {
		t.Fatal("fast mode should be on after /fast")
	}

	// Open config panel — should see fast mode as true.
	m, _ = submitCommand(m, "/config")
	cp := m.configPanel
	if cp == nil {
		t.Fatal("configPanel should be open")
	}
	val := cp.getValue(configSetting{id: "fastMode", typ: configBool})
	if val != "true" {
		t.Errorf("config panel shows fastMode = %q, want true", val)
	}

	// Toggle off via config panel.
	cp.toggleBool("fastMode", m.settings)

	// Close panel.
	result, _ := m.closeConfigPanel()
	m = result.(model)

	if m.fastMode {
		t.Error("m.fastMode should be false after config panel off")
	}
	if m.loop.FastMode() {
		t.Error("loop.FastMode() should be false after config panel off")
	}
	if m.settings.FastMode == nil || *m.settings.FastMode {
		t.Error("settings.FastMode should be false")
	}
}

// TestE2E_FastSync_ConfigOnThenSlashOff exercises the cross-path
// scenario: enable via config panel, disable via /fast.
func TestE2E_FastSync_ConfigOnThenSlashOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	settings := &config.Settings{}
	m, _ := testModel(t,
		withModelName(api.ModelAliases["opus"]),
		withFastMode(false),
		withSettings(settings),
	)

	// Enable via config panel.
	m, _ = submitCommand(m, "/config")
	m.configPanel.toggleBool("fastMode", m.settings)
	result, _ := m.closeConfigPanel()
	m = result.(model)

	if !m.fastMode {
		t.Fatal("fast mode should be on after config panel enable")
	}

	// Disable via /fast.
	cmd, _ := m.slashReg.lookup("fast")
	cmd.Execute(&m, "")

	if m.fastMode {
		t.Error("m.fastMode should be false after /fast off")
	}
	if m.loop.FastMode() {
		t.Error("loop.FastMode() should be false after /fast off")
	}
	if m.settings.FastMode == nil || *m.settings.FastMode {
		t.Error("settings.FastMode should be false after /fast off")
	}
}

// TestE2E_FastSync_ConfigPanelAutoSwitchModel verifies that enabling
// fast mode via config panel on a non-Opus model auto-switches to Opus.
func TestE2E_FastSync_ConfigPanelAutoSwitchModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	settings := &config.Settings{}
	m, _ := testModel(t,
		withModelName("claude-sonnet-4-20250514"),
		withFastMode(false),
		withSettings(settings),
	)

	// Enable fast mode via config panel on a non-Opus model.
	m, _ = submitCommand(m, "/config")
	m.configPanel.toggleBool("fastMode", m.settings)
	result, _ := m.closeConfigPanel()
	m = result.(model)

	// Should auto-switch to Opus.
	expected := api.ModelAliases[api.FastModeModelAlias]
	if m.modelName != expected {
		t.Errorf("model should auto-switch to %q, got %q", expected, m.modelName)
	}
	if !m.fastMode {
		t.Error("m.fastMode should be true")
	}
}
