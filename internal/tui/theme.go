package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds all TUI styles and colors. Keeping them centralised makes
// it straightforward to add dark/light mode support later.
var (
	// Colors.
	colorPurple  = lipgloss.Color("#A855F7")
	colorGreen   = lipgloss.Color("#22C55E")
	colorRed     = lipgloss.Color("#EF4444")
	colorYellow  = lipgloss.Color("#EAB308")
	colorDim     = lipgloss.Color("#6B7280")
	colorCyan    = lipgloss.Color("#06B6D4")
	colorWhite   = lipgloss.Color("#F9FAFB")

	// Prompt styles.
	promptStyle = lipgloss.NewStyle().
			Foreground(colorPurple).
			Bold(true)

	// Tool display.
	toolBulletStyle = lipgloss.NewStyle().
			Foreground(colorCyan).
			Bold(true)

	toolNameStyle = lipgloss.NewStyle().
			Foreground(colorCyan)

	toolSummaryStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	// Diff styles.
	diffAddStyle = lipgloss.NewStyle().
			Foreground(colorGreen)

	diffRemoveStyle = lipgloss.NewStyle().
			Foreground(colorRed)

	// Permission prompt.
	permTitleStyle = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)

	permToolStyle = lipgloss.NewStyle().
			Foreground(colorWhite).
			Bold(true)

	permHintStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	// Status bar.
	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	statusModelStyle = lipgloss.NewStyle().
				Foreground(colorPurple)

	// Error display.
	errorStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	// Todo item icons.
	todoPendingStyle    = lipgloss.NewStyle().Foreground(colorDim)
	todoInProgressStyle = lipgloss.NewStyle().Foreground(colorYellow)
	todoCompletedStyle  = lipgloss.NewStyle().Foreground(colorGreen)

	// AskUser question styles.
	askHeaderStyle   = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	askQuestionStyle = lipgloss.NewStyle().Foreground(colorWhite)
	askOptionStyle   = lipgloss.NewStyle().Foreground(colorDim)
	askSelectedStyle = lipgloss.NewStyle().Foreground(colorPurple).Bold(true)

	// User input echo.
	userLabelStyle = lipgloss.NewStyle().
			Foreground(colorPurple).
			Bold(true)
)
