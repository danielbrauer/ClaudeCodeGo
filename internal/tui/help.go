package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Help screen tab indices.
const (
	helpTabGeneral        = 0
	helpTabCommands       = 1
	helpTabCustomCommands = 2
	helpTabCount          = 3
)

// helpTabNames are the display labels for each tab.
var helpTabNames = [helpTabCount]string{"general", "commands", "custom-commands"}

// shortcutEntry represents a single shortcut line in the help screen.
type shortcutEntry struct {
	Key         string
	Description string
}

// handleHelpKey processes key events while the help screen is open.
func (m model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape, tea.KeyCtrlC:
		m.mode = modeInput
		m.textInput.Focus()
		return m, textarea.Blink

	case tea.KeyLeft:
		if m.helpTab > 0 {
			m.helpTab--
			m.helpScrollOff = 0
		}
		return m, nil

	case tea.KeyRight:
		if m.helpTab < helpTabCount-1 {
			m.helpTab++
			m.helpScrollOff = 0
		}
		return m, nil

	case tea.KeyTab:
		m.helpTab = (m.helpTab + 1) % helpTabCount
		m.helpScrollOff = 0
		return m, nil

	case tea.KeyShiftTab:
		m.helpTab = (m.helpTab + helpTabCount - 1) % helpTabCount
		m.helpScrollOff = 0
		return m, nil

	case tea.KeyUp:
		if m.helpScrollOff > 0 {
			m.helpScrollOff--
		}
		return m, nil

	case tea.KeyDown:
		m.helpScrollOff++
		return m, nil

	case tea.KeyPgUp:
		vp := m.helpViewportHeight()
		m.helpScrollOff -= vp
		if m.helpScrollOff < 0 {
			m.helpScrollOff = 0
		}
		return m, nil

	case tea.KeyPgDown:
		m.helpScrollOff += m.helpViewportHeight()
		return m, nil

	default:
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'q':
				// 'q' closes the help screen.
				m.mode = modeInput
				m.textInput.Focus()
				return m, textarea.Blink
			case 'j':
				m.helpScrollOff++
				return m, nil
			case 'k':
				if m.helpScrollOff > 0 {
					m.helpScrollOff--
				}
				return m, nil
			}
		}
		return m, nil
	}
}

// helpHeaderLines is the number of fixed lines above the scrollable content:
// title, blank, tab bar, blank.
const helpHeaderLines = 4

// helpFooterLines is the number of fixed lines below the scrollable content:
// blank, docs URL, blank, close hint.
const helpFooterLines = 4

// helpViewportHeight returns the number of visible content lines available for
// the scrollable tab content area.
func (m model) helpViewportHeight() int {
	h := m.height - helpHeaderLines - helpFooterLines
	if h < 1 {
		h = 1
	}
	return h
}

// Styles for the help screen.
var (
	helpTitleStyle = lipgloss.NewStyle().
			Foreground(colorCyan).
			Bold(true)

	helpTabActiveStyle = lipgloss.NewStyle().
				Foreground(colorWhite).
				Bold(true).
				Underline(true)

	helpTabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	helpSectionHeaderStyle = lipgloss.NewStyle().
				Bold(true)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	helpFooterStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Italic(true)
)

// renderHelpScreen renders the full help screen with scrolling support.
// The title and tab bar are always visible at the top, the footer is always
// visible at the bottom, and the tab content scrolls between them.
func (m model) renderHelpScreen() string {
	var b strings.Builder

	// ── Fixed header ──
	title := fmt.Sprintf("Claude Code v%s", m.version)
	b.WriteString(helpTitleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(m.renderHelpTabs())
	b.WriteString("\n\n")

	// ── Tab content (scrollable) ──
	var content string
	switch m.helpTab {
	case helpTabGeneral:
		content = m.renderHelpGeneral()
	case helpTabCommands:
		content = m.renderHelpCommands()
	case helpTabCustomCommands:
		content = m.renderHelpCustomCommands()
	}

	// Split content into lines for scrolling.
	lines := strings.Split(content, "\n")
	// Remove trailing empty line from the split if the content ended with \n.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	totalLines := len(lines)
	vpHeight := m.helpViewportHeight()

	// Clamp scroll offset.
	maxScroll := totalLines - vpHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.helpScrollOff > maxScroll {
		m.helpScrollOff = maxScroll
	}
	if m.helpScrollOff < 0 {
		m.helpScrollOff = 0
	}

	needsScroll := totalLines > vpHeight

	// Show "more above" indicator or blank line.
	if needsScroll && m.helpScrollOff > 0 {
		b.WriteString(helpFooterStyle.Render(fmt.Sprintf("  ↑ %d more lines above", m.helpScrollOff)))
		b.WriteString("\n")
	}

	// Render visible content lines.
	end := m.helpScrollOff + vpHeight
	if end > totalLines {
		end = totalLines
	}
	for i := m.helpScrollOff; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}

	// Show "more below" indicator.
	if needsScroll && end < totalLines {
		b.WriteString(helpFooterStyle.Render(fmt.Sprintf("  ↓ %d more lines below", totalLines-end)))
		b.WriteString("\n")
	}

	// ── Fixed footer ──
	b.WriteString("\n")
	b.WriteString("For more help: https://code.claude.com/docs/en/overview")
	b.WriteString("\n\n")
	hint := "esc to close"
	if needsScroll {
		hint = "↑↓/jk to scroll  esc to close"
	}
	b.WriteString(helpFooterStyle.Render(hint))

	return b.String()
}

// renderHelpTabs renders the tab bar with the active tab highlighted.
func (m model) renderHelpTabs() string {
	var parts []string
	for i, name := range helpTabNames {
		if i == m.helpTab {
			parts = append(parts, helpTabActiveStyle.Render(name))
		} else {
			parts = append(parts, helpTabInactiveStyle.Render(name))
		}
	}
	return strings.Join(parts, "  ")
}

// renderHelpGeneral renders the "general" tab content.
func (m model) renderHelpGeneral() string {
	var b strings.Builder

	b.WriteString("Claude understands your codebase, makes edits with your\n")
	b.WriteString("permission, and executes commands — right from your terminal.\n")
	b.WriteString("\n")

	b.WriteString(helpSectionHeaderStyle.Render("Shortcuts"))
	b.WriteString("\n\n")

	// Build the three columns of shortcuts.
	col1 := helpInputPrefixes()
	col2 := helpInputModifiers()
	col3 := helpChatShortcuts()

	// Calculate column widths for alignment.
	maxRows := len(col1)
	if len(col2) > maxRows {
		maxRows = len(col2)
	}
	if len(col3) > maxRows {
		maxRows = len(col3)
	}

	// Pad columns to equal length.
	for len(col1) < maxRows {
		col1 = append(col1, shortcutEntry{})
	}
	for len(col2) < maxRows {
		col2 = append(col2, shortcutEntry{})
	}
	for len(col3) < maxRows {
		col3 = append(col3, shortcutEntry{})
	}

	// Calculate column widths.
	col1Width := maxEntryWidth(col1)
	col2Width := maxEntryWidth(col2)
	if col1Width < 20 {
		col1Width = 20
	}
	if col2Width < 30 {
		col2Width = 30
	}

	// Render columns side by side.
	for i := 0; i < maxRows; i++ {
		line := formatShortcutEntry(col1[i], col1Width)
		line += "  "
		line += formatShortcutEntry(col2[i], col2Width)
		line += "  "
		line += formatShortcutEntry(col3[i], 0)
		b.WriteString(strings.TrimRight(line, " "))
		b.WriteString("\n")
	}

	return b.String()
}

// renderHelpCommands renders the "commands" tab content.
func (m model) renderHelpCommands() string {
	var b strings.Builder
	b.WriteString("Browse default commands:\n\n")

	cmds := m.slashReg.visibleCommands()
	// Filter to only built-in (non-skill) commands.
	var builtIn []SlashCommand
	for _, cmd := range cmds {
		if !cmd.IsSkill {
			builtIn = append(builtIn, cmd)
		}
	}

	for _, cmd := range builtIn {
		b.WriteString(fmt.Sprintf("  /%-14s %s\n", cmd.Name, cmd.Description))
	}

	if len(builtIn) == 0 {
		b.WriteString(helpDescStyle.Render("  No commands available"))
	}

	return b.String()
}

// renderHelpCustomCommands renders the "custom-commands" tab content.
func (m model) renderHelpCustomCommands() string {
	var b strings.Builder
	b.WriteString("Browse custom commands:\n\n")

	// Custom commands come from skills registered via registerSkills.
	var custom []SlashCommand
	for _, name := range m.slashReg.names {
		cmd := m.slashReg.commands[name]
		if cmd.IsAlias || cmd.IsHidden {
			continue
		}
		if cmd.IsSkill {
			custom = append(custom, cmd)
		}
	}

	if len(custom) == 0 {
		b.WriteString(helpDescStyle.Render("  No custom commands found"))
	} else {
		for _, cmd := range custom {
			b.WriteString(fmt.Sprintf("  /%-14s %s\n", cmd.Name, cmd.Description))
		}
	}

	return b.String()
}

// helpInputPrefixes returns the first column of shortcuts (input prefixes).
func helpInputPrefixes() []shortcutEntry {
	return []shortcutEntry{
		{Key: "!", Description: "for bash mode"},
		{Key: "/", Description: "for commands"},
		{Key: "@", Description: "for file paths"},
		{Key: "&", Description: "for background"},
	}
}

// helpInputModifiers returns the second column of shortcuts.
func helpInputModifiers() []shortcutEntry {
	return []shortcutEntry{
		{Key: "double tap esc", Description: "to clear input"},
		{Key: "shift+tab", Description: "to auto-accept edits"},
		{Key: "ctrl+o", Description: "for verbose output"},
		{Key: "ctrl+t", Description: "to toggle tasks"},
	}
}

// helpChatShortcuts returns the third column of shortcuts.
func helpChatShortcuts() []shortcutEntry {
	return []shortcutEntry{
		{Key: "ctrl+_", Description: "to undo"},
		{Key: "ctrl+z", Description: "to suspend"},
		{Key: "alt+p", Description: "to switch model"},
		{Key: "alt+o", Description: "to toggle fast mode"},
		{Key: "ctrl+s", Description: "to stash prompt"},
		{Key: "ctrl+g", Description: "to edit in $EDITOR"},
	}
}

// maxEntryWidth returns the max rendered width of shortcut entries.
func maxEntryWidth(entries []shortcutEntry) int {
	max := 0
	for _, e := range entries {
		w := len(e.Key) + 1 + len(e.Description)
		if w > max {
			max = w
		}
	}
	return max
}

// formatShortcutEntry formats a shortcut entry padded to the given width.
func formatShortcutEntry(e shortcutEntry, width int) string {
	if e.Key == "" && e.Description == "" {
		if width > 0 {
			return strings.Repeat(" ", width)
		}
		return ""
	}
	text := helpKeyStyle.Render(e.Key + " " + e.Description)
	// Pad with spaces. Note: ANSI escapes make len() unreliable for visible
	// width, so we pad based on the raw content length.
	rawLen := len(e.Key) + 1 + len(e.Description)
	if width > rawLen {
		text += strings.Repeat(" ", width-rawLen)
	}
	return text
}
