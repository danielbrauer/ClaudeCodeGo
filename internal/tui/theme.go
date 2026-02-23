package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds all TUI styles and colors. Keeping them centralised makes
// it straightforward to add dark/light mode support later.
var (
	// Colors.
	colorPurple   = lipgloss.Color("#A855F7")
	colorGreen    = lipgloss.Color("#22C55E")
	colorRed      = lipgloss.Color("#EF4444")
	colorYellow   = lipgloss.Color("#EAB308")
	colorDim      = lipgloss.Color("#6B7280")
	colorCyan     = lipgloss.Color("#06B6D4")
	colorWhite    = lipgloss.Color("#F9FAFB")
	colorOrange   = lipgloss.Color("#FF6A00")

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

	// Fast mode indicator.
	fastModeStyle = lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true)

	// User input echo.
	userLabelStyle = lipgloss.NewStyle().
			Foreground(colorPurple).
			Bold(true)

	// Resume session picker.
	resumeHeaderStyle = lipgloss.NewStyle().
				Foreground(colorCyan).
				Bold(true)

	resumeIDStyle = lipgloss.NewStyle().
			Foreground(colorPurple)

	// Diff dialog styles.
	diffTitleStyle = lipgloss.NewStyle().
			Foreground(colorWhite).
			Bold(true)

	diffDimStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	diffSelectedStyle = lipgloss.NewStyle().
				Foreground(colorPurple).
				Bold(true)

	diffFileHeaderStyle = lipgloss.NewStyle().
				Foreground(colorWhite).
				Bold(true)

	diffHunkHeaderStyle = lipgloss.NewStyle().
				Foreground(colorCyan)

	// Input border — the horizontal lines above and below the input area.
	// Matches the JS "promptBorder" color (dim gray).
	inputBorderStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	// Shortcuts hint shown below the input area.
	shortcutsHintStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	// Queued message styles.
	queuedLabelStyle = lipgloss.NewStyle().
				Foreground(colorDim).
				Bold(true)

	queuedBadgeStyle = lipgloss.NewStyle().
				Foreground(colorYellow)
)
