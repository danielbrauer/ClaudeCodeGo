package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// registerUICommands registers /help, /config, /settings, /model, /diff, /memory.
func registerUICommands(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "help",
		Description: "Show help and available commands",
		Execute:     executeHelp,
	})

	r.register(SlashCommand{
		Name:        "config",
		Description: "Open config panel",
		Execute:     executeConfig,
	})

	r.register(SlashCommand{
		Name:        "settings",
		Description: "Open config panel",
		IsAlias:     true,
		Execute:     executeConfig,
	})

	r.register(SlashCommand{
		Name:        "model",
		Description: "Show or switch model",
		Execute:     executeModel,
	})

	r.register(SlashCommand{
		Name:        "diff",
		Description: "View uncommitted changes",
		Execute:     executeDiff,
	})

	r.register(SlashCommand{
		Name:        "memory",
		Description: "Edit Claude memory files",
		Execute:     executeMemory,
	})
}

func executeHelp(m *model, args string) (tea.Model, tea.Cmd) {
	m.helpTab = 0
	m.helpScrollOff = 0
	m.mode = modeHelp
	m.textInput.Blur()
	return *m, nil
}

func executeConfig(m *model, args string) (tea.Model, tea.Cmd) {
	if m.settings != nil {
		m.configPanel = newConfigPanel(m.settings)
		m.mode = modeConfig
		m.textInput.Blur()
		return *m, nil
	}
	return *m, tea.Println(errorStyle.Render("No settings loaded."))
}

func executeModel(m *model, args string) (tea.Model, tea.Cmd) {
	parts := []string{"model"}
	if args != "" {
		parts = append(parts, args)
	}
	return m.handleModelCommand(parts)
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
