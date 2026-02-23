package tui

import (
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
)

func TestE2E_ModelCommand_NoArg_OpensModelPicker(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/model")

	if result.mode != modeModelPicker {
		t.Errorf("mode = %d, want modeModelPicker (%d)", result.mode, modeModelPicker)
	}
}

func TestE2E_ModelCommand_WithAlias(t *testing.T) {
	var switchedTo string
	m, _ := testModel(t,
		withModelName("claude-sonnet-4-20250514"),
		withOnModelSwitch(func(newModel string) {
			switchedTo = newModel
		}),
	)

	result, _ := submitCommand(m, "/model opus")

	// Should have switched model.
	expected := api.ResolveModelAlias("opus")
	if result.modelName != expected {
		t.Errorf("modelName = %q, want %q", result.modelName, expected)
	}
	if switchedTo != expected {
		t.Errorf("onModelSwitch called with %q, want %q", switchedTo, expected)
	}
}

func TestE2E_ModelCommand_WithFullID(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/model claude-haiku-4-5-20251001")

	if result.modelName != "claude-haiku-4-5-20251001" {
		t.Errorf("modelName = %q, want claude-haiku-4-5-20251001", result.modelName)
	}
}

func TestE2E_ModelCommand_PreSelectsCurrent(t *testing.T) {
	// Set the current model to the second available model.
	secondModel := api.AvailableModels[1].ID
	m, _ := testModel(t, withModelName(secondModel))

	result, _ := submitCommand(m, "/model")

	if result.mode != modeModelPicker {
		t.Fatalf("mode = %d, want modeModelPicker", result.mode)
	}
	if result.modelPickerCursor != 1 {
		t.Errorf("modelPickerCursor = %d, want 1 (pre-selected current model)", result.modelPickerCursor)
	}
}

func TestE2E_ModelCommand_UpdatesLoop(t *testing.T) {
	m, _ := testModel(t, withModelName("claude-sonnet-4-20250514"))

	result, _ := submitCommand(m, "/model haiku")

	// Verify the loop's model was also updated.
	expected := api.ResolveModelAlias("haiku")
	if result.modelName != expected {
		t.Errorf("modelName = %q, want %q", result.modelName, expected)
	}
}
