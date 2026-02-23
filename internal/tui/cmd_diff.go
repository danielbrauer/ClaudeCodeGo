package tui

import tea "github.com/charmbracelet/bubbletea"

// registerDiffCommand registers /diff.
func registerDiffCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "diff",
		Description: "View uncommitted changes",
		Execute:     executeDiff,
	})
}

func executeDiff(m *model, args string) (tea.Model, tea.Cmd) {
	m.mode = modeDiff
	m.diffData = nil
	return *m, tea.Batch(
		func() tea.Msg {
			data := loadDiffData()
			return DiffLoadedMsg{Data: data}
		},
		m.spinner.Tick,
	)
}
