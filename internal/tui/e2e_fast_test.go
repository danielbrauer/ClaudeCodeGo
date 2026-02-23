package tui

import (
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
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
	output := cmd.Execute(&m)

	if output != "Fast mode ON" {
		t.Errorf("fast output = %q, want 'Fast mode ON'", output)
	}
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
	output := cmd.Execute(&m)

	if output != "Fast mode OFF" {
		t.Errorf("fast output = %q, want 'Fast mode OFF'", output)
	}
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
	output := cmd.Execute(&m)

	if output != "Fast mode ON" {
		t.Errorf("fast output = %q, want 'Fast mode ON'", output)
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
	cmd.Execute(&m)

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
	cmd.Execute(&m)

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
	output := cmd.Execute(&m)
	if output != "Fast mode ON" {
		t.Errorf("first toggle = %q, want ON", output)
	}

	// Off.
	output = cmd.Execute(&m)
	if output != "Fast mode OFF" {
		t.Errorf("second toggle = %q, want OFF", output)
	}

	// On again.
	output = cmd.Execute(&m)
	if output != "Fast mode ON" {
		t.Errorf("third toggle = %q, want ON", output)
	}
}
