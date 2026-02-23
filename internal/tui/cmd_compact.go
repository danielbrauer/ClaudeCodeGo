package tui

import tea "github.com/charmbracelet/bubbletea"

// registerCompactCommand registers /compact.
func registerCompactCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "compact",
		Description: "Compact conversation history",
		Execute:     executeCompact,
	})
}

func executeCompact(m *model, args string) (tea.Model, tea.Cmd) {
	m.mode = modeStreaming
	return *m, func() tea.Msg {
		err := m.loop.Compact(m.ctx)
		if err != nil {
			return LoopDoneMsg{Err: err}
		}
		return LoopDoneMsg{}
	}
}
