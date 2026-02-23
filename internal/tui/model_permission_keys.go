package tui

import tea "github.com/charmbracelet/bubbletea"

// handlePermissionKey processes key events during a permission prompt.
func (m model) handlePermissionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.permissionPending == nil {
		m.mode = modeInput
		return m, nil
	}

	var cmds []tea.Cmd

	switch msg.String() {
	case "y", "Y":
		m.permissionPending.ResultCh <- PermissionAllow
		line := renderPermissionResultLine(m.permissionPending.ToolName, m.permissionPending.Summary, PermissionAllow)
		cmds = append(cmds, tea.Println(line))
		m.permissionPending = nil
		m.mode = modeStreaming
		return m, tea.Batch(cmds...)

	case "n", "N":
		m.permissionPending.ResultCh <- PermissionDeny
		line := renderPermissionResultLine(m.permissionPending.ToolName, m.permissionPending.Summary, PermissionDeny)
		cmds = append(cmds, tea.Println(line))
		m.permissionPending = nil
		m.mode = modeStreaming
		return m, tea.Batch(cmds...)

	case "a", "A":
		// "Always allow" — only available when suggestions exist.
		if len(m.permissionPending.Suggestions) > 0 {
			m.permissionPending.ResultCh <- PermissionAlwaysAllow
			line := renderPermissionResultLine(m.permissionPending.ToolName, m.permissionPending.Summary, PermissionAlwaysAllow)
			cmds = append(cmds, tea.Println(line))
			m.permissionPending = nil
			m.mode = modeStreaming
			return m, tea.Batch(cmds...)
		}

	case "ctrl+c":
		m.permissionPending.ResultCh <- PermissionDeny
		m.permissionPending = nil
		m.cancelFn()
		return m, nil
	}

	return m, nil
}
