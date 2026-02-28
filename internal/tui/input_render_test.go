package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// TestTextInputRendersTypedCharacters verifies that typed characters appear
// in the View() output. This reproduces the bug where the cursor is visible
// but typed characters are not displayed.
func TestTextInputRendersTypedCharacters(t *testing.T) {
	m, _ := testModel(t)

	// Verify initial state: modeInput, focused
	if m.mode != modeInput {
		t.Fatalf("expected modeInput, got %d", m.mode)
	}
	if !m.textInput.Focused() {
		t.Fatal("textarea should be focused initially")
	}

	// Initial view should render the input area
	view := m.View()
	if !strings.Contains(view, "❯") {
		t.Fatal("initial view should contain the prompt chevron")
	}

	// Type 'h' — simulate a key press through the full Update path
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	result, _ := m.Update(keyMsg)
	m = result.(model)

	// Verify the textarea has the character
	if m.textInput.Value() != "h" {
		t.Fatalf("expected textInput value 'h', got %q", m.textInput.Value())
	}

	// Verify the character appears in the View
	view = m.View()
	stripped := ansi.Strip(view)
	if !strings.Contains(stripped, "h") {
		t.Fatalf("view should contain typed character 'h', got:\n%s", stripped)
	}

	// Type more characters to spell "hello"
	for _, ch := range []rune{'e', 'l', 'l', 'o'} {
		keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		result, _ = m.Update(keyMsg)
		m = result.(model)
	}

	if m.textInput.Value() != "hello" {
		t.Fatalf("expected textInput value 'hello', got %q", m.textInput.Value())
	}

	view = m.View()
	stripped = ansi.Strip(view)
	if !strings.Contains(stripped, "hello") {
		t.Fatalf("view should contain 'hello', got:\n%s", stripped)
	}

	// Verify the textarea is still focused
	if !m.textInput.Focused() {
		t.Fatal("textarea should still be focused after typing")
	}
}

// TestTextInputViewLineCountStability checks that the View() output has a
// stable line count between empty and typed states, which is critical for
// Bubble Tea's inline renderer.
func TestTextInputViewLineCountStability(t *testing.T) {
	m, _ := testModel(t)

	// Count lines in the initial view (empty input)
	initialView := m.View()
	initialLines := strings.Count(initialView, "\n")
	t.Logf("Initial view lines: %d", initialLines)
	t.Logf("Initial view (stripped):\n%s", ansi.Strip(initialView))

	// Type a character and count lines
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	result, _ := m.Update(keyMsg)
	m = result.(model)

	typedView := m.View()
	typedLines := strings.Count(typedView, "\n")
	t.Logf("After typing 'h' - lines: %d", typedLines)
	t.Logf("After typing view (stripped):\n%s", ansi.Strip(typedView))

	// The line count should be the same (stable rendering)
	if initialLines != typedLines {
		t.Errorf("view line count changed: %d (empty) vs %d (typed) — this can cause rendering glitches", initialLines, typedLines)
	}
}

// TestTextInputRenderAfterLoopDone verifies that the textarea renders correctly
// after the agentic loop completes and the mode returns to modeInput.
func TestTextInputRenderAfterLoopDone(t *testing.T) {
	m, _ := testModel(t)

	// Simulate a loop completion (as if we just finished processing a message)
	loopDoneResult, _ := m.handleLoopDone(LoopDoneMsg{})
	m = loopDoneResult.(model)

	// Verify we're back in input mode
	if m.mode != modeInput {
		t.Fatalf("expected modeInput after loop done, got %d", m.mode)
	}

	// Verify textarea is focused
	if !m.textInput.Focused() {
		t.Fatal("textarea should be focused after loop done")
	}

	// Type characters and verify rendering
	for _, ch := range []rune{'t', 'e', 's', 't'} {
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		result, _ := m.Update(keyMsg)
		m = result.(model)
	}

	if m.textInput.Value() != "test" {
		t.Fatalf("expected textInput value 'test', got %q", m.textInput.Value())
	}

	view := m.View()
	stripped := ansi.Strip(view)
	if !strings.Contains(stripped, "test") {
		t.Fatalf("view should contain 'test' after loop done, got:\n%s", stripped)
	}
}

// TestTextInputViewDebug dumps the exact View() output for debugging.
func TestTextInputViewDebug(t *testing.T) {
	m, _ := testModel(t)

	t.Log("=== EMPTY INPUT VIEW ===")
	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		t.Logf("  line %d: %q (stripped: %q)", i, line, ansi.Strip(line))
	}

	// Type hello
	for _, ch := range []rune{'h', 'e', 'l', 'l', 'o'} {
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = result.(model)
	}

	t.Log("=== AFTER TYPING 'hello' VIEW ===")
	view = m.View()
	for i, line := range strings.Split(view, "\n") {
		t.Logf("  line %d: %q (stripped: %q)", i, line, ansi.Strip(line))
	}

	// Check textarea View() directly
	t.Log("=== TEXTAREA VIEW DIRECTLY ===")
	taView := m.textInput.View()
	for i, line := range strings.Split(taView, "\n") {
		t.Logf("  line %d: %q (stripped: %q)", i, line, ansi.Strip(line))
	}

	// Now check the textarea value vs what's rendered
	t.Logf("textInput.Value() = %q", m.textInput.Value())
	t.Logf("textInput.Focused() = %v", m.textInput.Focused())
	t.Logf("textInput.Height() = %d", m.textInput.Height())
	t.Logf("textInput.Width() = %d", m.textInput.Width())

	// Verify that the rendered textarea contains the typed text
	strippedTA := ansi.Strip(taView)
	if !strings.Contains(strippedTA, "hello") {
		t.Errorf("textarea View() does not contain 'hello': %q", strippedTA)
	}

	_ = fmt.Sprintf // avoid unused import
}
