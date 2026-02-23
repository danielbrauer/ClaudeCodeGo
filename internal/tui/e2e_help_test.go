package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestE2E_HelpCommand_OpensHelpScreen(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/help")

	// After /help, model should switch to modeHelp.
	if result.mode != modeHelp {
		t.Errorf("mode = %d, want modeHelp (%d)", result.mode, modeHelp)
	}
	if result.helpTab != helpTabGeneral {
		t.Errorf("helpTab = %d, want helpTabGeneral (%d)", result.helpTab, helpTabGeneral)
	}
}

func TestE2E_HelpCommand_Registered(t *testing.T) {
	m, _ := testModel(t)

	_, ok := m.slashReg.lookup("help")
	if !ok {
		t.Fatal("/help not registered")
	}
}

func TestE2E_HelpScreen_GeneralTab(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral

	view := m.renderHelpScreen()

	// Should contain the title with version.
	if !strings.Contains(view, "Claude Code v") {
		t.Error("help screen should contain title with version")
	}

	// Should contain the description.
	if !strings.Contains(view, "Claude understands your codebase") {
		t.Error("help screen should contain description")
	}

	// Should contain the Shortcuts header.
	if !strings.Contains(view, "Shortcuts") {
		t.Error("help screen should contain Shortcuts header")
	}

	// Should contain input prefix shortcuts.
	for _, shortcut := range []string{"for bash mode", "for commands", "for file paths", "for background"} {
		if !strings.Contains(view, shortcut) {
			t.Errorf("help screen should contain shortcut: %s", shortcut)
		}
	}

	// Should contain keyboard shortcuts.
	for _, shortcut := range []string{"to undo", "to switch model", "to toggle fast mode", "to edit in $EDITOR"} {
		if !strings.Contains(view, shortcut) {
			t.Errorf("help screen should contain shortcut: %s", shortcut)
		}
	}

	// Should contain the footer.
	if !strings.Contains(view, "https://code.claude.com/docs/en/overview") {
		t.Error("help screen should contain docs URL")
	}
	if !strings.Contains(view, "esc to close") {
		t.Error("help screen should contain close hint")
	}
}

func TestE2E_HelpScreen_CommandsTab(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabCommands

	view := m.renderHelpScreen()

	// Should contain the browse header.
	if !strings.Contains(view, "Browse default commands:") {
		t.Error("commands tab should contain 'Browse default commands:'")
	}

	// Should list key commands.
	for _, name := range []string{"/help", "/model", "/cost", "/context", "/compact", "/clear", "/memory", "/init", "/quit"} {
		if !strings.Contains(view, name) {
			t.Errorf("commands tab should list %s", name)
		}
	}

	// Aliases should not appear.
	for _, alias := range []string{"/exit ", "/reset ", "/new ", "/settings "} {
		if strings.Contains(view, alias) {
			t.Errorf("commands tab should not list alias %s", strings.TrimSpace(alias))
		}
	}
}

func TestE2E_HelpScreen_CustomCommandsTab(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabCustomCommands

	view := m.renderHelpScreen()

	// Should contain the browse header.
	if !strings.Contains(view, "Browse custom commands:") {
		t.Error("custom commands tab should contain 'Browse custom commands:'")
	}

	// With no skills registered, should show empty message.
	if !strings.Contains(view, "No custom commands found") {
		t.Error("custom commands tab should show 'No custom commands found' when empty")
	}
}

func TestE2E_HelpScreen_TabNavigation(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral

	// Tab key should cycle forward.
	result, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyTab})
	rm := result.(model)
	if rm.helpTab != helpTabCommands {
		t.Errorf("after Tab, helpTab = %d, want %d", rm.helpTab, helpTabCommands)
	}

	// Right arrow should advance.
	result, _ = rm.handleHelpKey(tea.KeyMsg{Type: tea.KeyRight})
	rm = result.(model)
	if rm.helpTab != helpTabCustomCommands {
		t.Errorf("after Right, helpTab = %d, want %d", rm.helpTab, helpTabCustomCommands)
	}

	// Right at the end should not go further.
	result, _ = rm.handleHelpKey(tea.KeyMsg{Type: tea.KeyRight})
	rm = result.(model)
	if rm.helpTab != helpTabCustomCommands {
		t.Errorf("after Right at end, helpTab = %d, want %d", rm.helpTab, helpTabCustomCommands)
	}

	// Left arrow should go back.
	result, _ = rm.handleHelpKey(tea.KeyMsg{Type: tea.KeyLeft})
	rm = result.(model)
	if rm.helpTab != helpTabCommands {
		t.Errorf("after Left, helpTab = %d, want %d", rm.helpTab, helpTabCommands)
	}

	// Shift+Tab should cycle backward.
	result, _ = rm.handleHelpKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	rm = result.(model)
	if rm.helpTab != helpTabGeneral {
		t.Errorf("after Shift+Tab, helpTab = %d, want %d", rm.helpTab, helpTabGeneral)
	}

	// Shift+Tab at the start should wrap around.
	result, _ = rm.handleHelpKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	rm = result.(model)
	if rm.helpTab != helpTabCustomCommands {
		t.Errorf("after Shift+Tab wrap, helpTab = %d, want %d", rm.helpTab, helpTabCustomCommands)
	}
}

func TestE2E_HelpScreen_EscCloses(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral

	result, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyEscape})
	rm := result.(model)
	if rm.mode != modeInput {
		t.Errorf("after Esc, mode = %d, want modeInput (%d)", rm.mode, modeInput)
	}
}

func TestE2E_HelpScreen_QCloses(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral

	result, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	rm := result.(model)
	if rm.mode != modeInput {
		t.Errorf("after q, mode = %d, want modeInput (%d)", rm.mode, modeInput)
	}
}

func TestE2E_HelpScreen_QuestionMarkOpensHelp(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeInput
	// Input should be empty to trigger help.
	m.textInput.Reset()

	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	rm := result.(model)
	if rm.mode != modeHelp {
		t.Errorf("pressing ? with empty input: mode = %d, want modeHelp (%d)", rm.mode, modeHelp)
	}
}

func TestE2E_HelpScreen_QuestionMarkWithTextDoesNotOpenHelp(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeInput
	m.textInput.SetValue("hello")

	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	rm := result.(model)
	if rm.mode != modeInput {
		t.Errorf("pressing ? with text: mode = %d, want modeInput (%d)", rm.mode, modeInput)
	}
}

func TestE2E_VisibleCommands_HidesAliases(t *testing.T) {
	m, _ := testModel(t)

	cmds := m.slashReg.visibleCommands()
	names := make(map[string]bool)
	for _, cmd := range cmds {
		names[cmd.Name] = true
	}

	// Aliases should be hidden.
	for _, alias := range []string{"exit", "reset", "new", "settings"} {
		if names[alias] {
			t.Errorf("visibleCommands should not include alias %q", alias)
		}
	}

	// Core commands should be visible.
	for _, name := range []string{"help", "model", "cost", "context", "compact", "clear", "memory", "init", "quit"} {
		if !names[name] {
			t.Errorf("visibleCommands should include %q", name)
		}
	}
}

func TestE2E_HelpScreen_TabBarRendered(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp

	for tab := 0; tab < helpTabCount; tab++ {
		m.helpTab = tab
		view := m.renderHelpTabs()

		// All tab names should appear.
		for _, name := range helpTabNames {
			if !strings.Contains(view, name) {
				t.Errorf("tab bar (active=%d) should contain tab name %q", tab, name)
			}
		}
	}
}
