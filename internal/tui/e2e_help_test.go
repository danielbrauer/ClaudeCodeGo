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
	m.height = 100 // large enough to show all commands without scrolling

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

func TestE2E_HelpScreen_ScrollDown(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral
	m.height = 12 // small terminal to force scrolling

	// Initial scroll offset should be 0.
	if m.helpScrollOff != 0 {
		t.Fatalf("initial helpScrollOff = %d, want 0", m.helpScrollOff)
	}

	// Pressing Down should increment scroll offset.
	result, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyDown})
	rm := result.(model)
	if rm.helpScrollOff != 1 {
		t.Errorf("after Down, helpScrollOff = %d, want 1", rm.helpScrollOff)
	}
}

func TestE2E_HelpScreen_ScrollUp(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral
	m.helpScrollOff = 3

	// Pressing Up should decrement scroll offset.
	result, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyUp})
	rm := result.(model)
	if rm.helpScrollOff != 2 {
		t.Errorf("after Up, helpScrollOff = %d, want 2", rm.helpScrollOff)
	}
}

func TestE2E_HelpScreen_ScrollUpClampsAtZero(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral
	m.helpScrollOff = 0

	// Up at offset 0 should stay at 0.
	result, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyUp})
	rm := result.(model)
	if rm.helpScrollOff != 0 {
		t.Errorf("after Up at 0, helpScrollOff = %d, want 0", rm.helpScrollOff)
	}
}

func TestE2E_HelpScreen_JKScroll(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral
	m.helpScrollOff = 0

	// 'j' should scroll down.
	result, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	rm := result.(model)
	if rm.helpScrollOff != 1 {
		t.Errorf("after j, helpScrollOff = %d, want 1", rm.helpScrollOff)
	}

	// 'k' should scroll up.
	result, _ = rm.handleHelpKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	rm = result.(model)
	if rm.helpScrollOff != 0 {
		t.Errorf("after k, helpScrollOff = %d, want 0", rm.helpScrollOff)
	}
}

func TestE2E_HelpScreen_TabSwitchResetsScroll(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral
	m.helpScrollOff = 5

	// Switching tabs should reset scroll offset.
	result, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyTab})
	rm := result.(model)
	if rm.helpScrollOff != 0 {
		t.Errorf("after Tab, helpScrollOff = %d, want 0", rm.helpScrollOff)
	}
	if rm.helpTab != helpTabCommands {
		t.Errorf("after Tab, helpTab = %d, want %d", rm.helpTab, helpTabCommands)
	}
}

func TestE2E_HelpScreen_ScrollClampsInRender(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral
	m.height = 24
	m.helpScrollOff = 9999 // way past the end

	// Rendering should clamp the offset and still show tab content.
	view := m.renderHelpScreen()
	if !strings.Contains(view, "Claude Code v") {
		t.Error("title should always be visible")
	}
	// Tab bar should always be visible.
	for _, name := range helpTabNames {
		if !strings.Contains(view, name) {
			t.Errorf("tab bar should be visible, missing %q", name)
		}
	}
	// Footer should always be visible.
	if !strings.Contains(view, "esc to close") {
		t.Error("footer should always be visible")
	}
}

func TestE2E_HelpScreen_ScrollIndicators(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabCommands
	m.height = 10 // very small to force scrolling (viewport = 10 - 8 = 2 lines)

	// At top, should show "more below" but not "more above".
	m.helpScrollOff = 0
	view := m.renderHelpScreen()
	if strings.Contains(view, "more lines above") {
		t.Error("at top, should not show 'more lines above'")
	}
	if !strings.Contains(view, "more lines below") {
		t.Error("at top with small terminal, should show 'more lines below'")
	}

	// Scroll down a bit — should show both indicators.
	m.helpScrollOff = 2
	view = m.renderHelpScreen()
	if !strings.Contains(view, "more lines above") {
		t.Error("after scrolling down, should show 'more lines above'")
	}
}

func TestE2E_HelpScreen_TabBarAlwaysVisible(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral
	m.height = 10 // small terminal
	m.helpScrollOff = 5

	view := m.renderHelpScreen()

	// Tab bar must always be present regardless of scroll position.
	if !strings.Contains(view, "general") {
		t.Error("tab bar should always show 'general' tab")
	}
	if !strings.Contains(view, "commands") {
		t.Error("tab bar should always show 'commands' tab")
	}
	if !strings.Contains(view, "custom-commands") {
		t.Error("tab bar should always show 'custom-commands' tab")
	}
}

func TestE2E_HelpScreen_PageUpDown(t *testing.T) {
	m, _ := testModel(t)
	m.mode = modeHelp
	m.helpTab = helpTabGeneral
	m.height = 12 // viewport = 12 - 8 = 4
	m.helpScrollOff = 0

	// PageDown should jump by viewport height.
	result, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyPgDown})
	rm := result.(model)
	if rm.helpScrollOff != 4 {
		t.Errorf("after PgDown, helpScrollOff = %d, want 4", rm.helpScrollOff)
	}

	// PageUp should jump back.
	result, _ = rm.handleHelpKey(tea.KeyMsg{Type: tea.KeyPgUp})
	rm = result.(model)
	if rm.helpScrollOff != 0 {
		t.Errorf("after PgUp, helpScrollOff = %d, want 0", rm.helpScrollOff)
	}
}
