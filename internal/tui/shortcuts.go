package tui

import (
	"fmt"
	"strings"
)

// shortcutEntry represents a single keyboard shortcut for display.
type shortcutEntry struct {
	key         string
	description string
}

// renderShortcutsHelp renders the keyboard shortcuts help panel,
// matching the layout shown in the official CLI when the user presses "?".
func renderShortcutsHelp(width int) string {
	col1 := []shortcutEntry{
		{"!", "bash mode"},
		{"/", "commands"},
		{"@", "file paths"},
	}

	col2 := []shortcutEntry{
		{"Esc Esc", "clear input"},
		{"Shift+Tab", "auto-accept edits"},
		{"Ctrl+T", "toggle tasks"},
		{"Shift+Enter", "new line"},
	}

	col3 := []shortcutEntry{
		{"Ctrl+C", "interrupt / quit"},
		{"Ctrl+G", "edit in $EDITOR"},
		{"/compact", "compact history"},
		{"/model", "switch model"},
		{"/help", "all commands"},
	}

	renderCol := func(entries []shortcutEntry) string {
		var b strings.Builder
		for _, e := range entries {
			line := fmt.Sprintf("  %s %s", permHintStyle.Render(e.key), permHintStyle.Render("→ "+e.description))
			b.WriteString(line + "\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}

	// For narrow terminals, render as a single column.
	if width < 70 {
		var all []shortcutEntry
		all = append(all, col1...)
		all = append(all, col2...)
		all = append(all, col3...)

		var b strings.Builder
		b.WriteString(permHintStyle.Render("  Shortcuts") + "\n")
		b.WriteString(renderCol(all))
		return b.String()
	}

	// For wider terminals, render in three columns side by side.
	lines1 := strings.Split(renderCol(col1), "\n")
	lines2 := strings.Split(renderCol(col2), "\n")
	lines3 := strings.Split(renderCol(col3), "\n")

	// Pad columns to equal height.
	maxLines := len(lines1)
	if len(lines2) > maxLines {
		maxLines = len(lines2)
	}
	if len(lines3) > maxLines {
		maxLines = len(lines3)
	}
	for len(lines1) < maxLines {
		lines1 = append(lines1, "")
	}
	for len(lines2) < maxLines {
		lines2 = append(lines2, "")
	}
	for len(lines3) < maxLines {
		lines3 = append(lines3, "")
	}

	colWidth := (width - 4) / 3
	if colWidth < 20 {
		colWidth = 20
	}

	var b strings.Builder
	b.WriteString(permHintStyle.Render("  Shortcuts") + "\n")
	for i := 0; i < maxLines; i++ {
		l1 := padRight(lines1[i], colWidth)
		l2 := padRight(lines2[i], colWidth)
		l3 := lines3[i]
		b.WriteString(l1 + l2 + l3 + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// padRight pads a string to at least width characters using spaces.
// It counts visible characters (approximation for ANSI-styled strings).
func padRight(s string, width int) string {
	// Use a simple approach: just pad the raw string.
	// Since we're using lipgloss styles that add ANSI codes,
	// we need to measure the visible width.
	visible := stripAnsi(s)
	if len(visible) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(visible))
}

// stripAnsi removes ANSI escape sequences to measure visible string width.
func stripAnsi(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
