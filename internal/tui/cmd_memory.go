package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// registerMemoryCommand registers /memory.
func registerMemoryCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "memory",
		Description: "Edit Claude memory files",
		Execute:     executeMemory,
	})
}

func executeMemory(m *model, args string) (tea.Model, tea.Cmd) {
	arg := strings.TrimSpace(args)
	cwd, _ := os.Getwd()
	filePath := memoryFilePath(arg, cwd)
	editorCmd, err := editorCommand(filePath)
	if err != nil {
		return *m, tea.Println("Error: " + err.Error())
	}
	execCb := func(err error) tea.Msg {
		return MemoryEditDoneMsg{Path: filePath, Err: err}
	}
	return *m, tea.Batch(tea.ExecProcess(editorCmd, execCb), textarea.Blink)
}
