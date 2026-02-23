package tui

import (
	"strings"
	"testing"
)

func TestE2E_CostCommand_ZeroTokens(t *testing.T) {
	m, _ := testModel(t)

	cmd, ok := m.slashReg.lookup("cost")
	if !ok {
		t.Fatal("/cost not registered")
	}
	output := cmd.Execute(&m)

	if !strings.Contains(output, "Input tokens:") {
		t.Error("cost output should contain 'Input tokens:'")
	}
	if !strings.Contains(output, "Output tokens:") {
		t.Error("cost output should contain 'Output tokens:'")
	}
	if !strings.Contains(output, "Cache read:") {
		t.Error("cost output should contain 'Cache read:'")
	}
	if !strings.Contains(output, "Cache write:") {
		t.Error("cost output should contain 'Cache write:'")
	}
	if !strings.Contains(output, "API turns:") {
		t.Error("cost output should contain 'API turns:'")
	}
}

func TestE2E_CostCommand_WithTokens(t *testing.T) {
	m, _ := testModel(t)

	// Simulate token usage.
	cacheRead := 500
	cacheWrite := 100
	m.tokens.addInput(1000, &cacheRead, &cacheWrite)
	m.tokens.addOutput(250)

	cmd, _ := m.slashReg.lookup("cost")
	output := cmd.Execute(&m)

	if !strings.Contains(output, "1000") {
		t.Errorf("cost output should contain input token count 1000, got %q", output)
	}
	if !strings.Contains(output, "250") {
		t.Errorf("cost output should contain output token count 250, got %q", output)
	}
	if !strings.Contains(output, "500") {
		t.Errorf("cost output should contain cache read count 500, got %q", output)
	}
	if !strings.Contains(output, "100") {
		t.Errorf("cost output should contain cache write count 100, got %q", output)
	}
}

func TestE2E_CostCommand_TracksTurns(t *testing.T) {
	m, _ := testModel(t)

	m.tokens.addOutput(100)
	m.tokens.addOutput(200)
	m.tokens.addOutput(50)

	cmd, _ := m.slashReg.lookup("cost")
	output := cmd.Execute(&m)

	// Should report 3 API turns.
	if !strings.Contains(output, "3") {
		t.Errorf("cost output should reflect 3 turns, got %q", output)
	}
}
