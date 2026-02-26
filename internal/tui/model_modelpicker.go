package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
)

// handleModelCommand processes /model with optional argument.
func (m model) handleModelCommand(parts []string) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if len(parts) < 2 {
		// No argument: open interactive model picker.
		m.modelPickerCursor = 0
		// Pre-select the current model.
		for i, opt := range api.AvailableModels {
			if opt.ID == m.modelName || opt.Alias == m.modelName {
				m.modelPickerCursor = i
				break
			}
		}
		m.mode = modeModelPicker
		return m, nil
	}

	// Argument provided: switch directly.
	arg := strings.TrimSpace(parts[1])
	resolved := api.ResolveModelAlias(arg)

	return m.switchModel(resolved, cmds)
}

// switchModel updates the model across the loop, TUI state, and session.
func (m model) switchModel(newModel string, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	m.loop.SetModel(newModel)
	m.modelName = newModel
	m.tokens.setModel(newModel)

	if m.onModelSwitch != nil {
		m.onModelSwitch(newModel)
	}

	display := api.ModelDisplayName(newModel)
	msg := fmt.Sprintf("Switched to model: %s (%s)", display, newModel)
	cmds = append(cmds, tea.Println(msg))
	return m, tea.Batch(cmds...)
}

// handleModelPickerKey processes key events during the model picker.
func (m model) handleModelPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	numOptions := len(api.AvailableModels)

	switch msg.Type {
	case tea.KeyUp:
		if m.modelPickerCursor > 0 {
			m.modelPickerCursor--
		}
		return m, nil

	case tea.KeyDown:
		if m.modelPickerCursor < numOptions-1 {
			m.modelPickerCursor++
		}
		return m, nil

	case tea.KeyEnter:
		selected := api.AvailableModels[m.modelPickerCursor]
		m.mode = modeInput
		m.textInput.Focus()
		return m.switchModel(selected.ID, []tea.Cmd{textarea.Blink})

	case tea.KeyEsc, tea.KeyCtrlC:
		m.mode = modeInput
		m.textInput.Focus()
		return m, tea.Println("Model selection cancelled.")
	}

	return m, nil
}

// renderModelPicker renders the model selection UI.
func (m model) renderModelPicker() string {
	var b strings.Builder

	b.WriteString(askHeaderStyle.Render("[Model]") + " " + askQuestionStyle.Render("Select a model:") + "\n")

	for i, opt := range api.AvailableModels {
		current := ""
		if opt.ID == m.modelName {
			current = " (current)"
		}
		if i == m.modelPickerCursor {
			b.WriteString(askSelectedStyle.Render(fmt.Sprintf("  > %s%s", opt.DisplayName, current)) + " " + askOptionStyle.Render(opt.Description) + "\n")
		} else {
			b.WriteString(askOptionStyle.Render(fmt.Sprintf("    %s%s %s", opt.DisplayName, current, opt.Description)) + "\n")
		}
	}

	b.WriteString(permHintStyle.Render("  Use arrow keys to navigate, Enter to select, Esc to cancel"))

	return b.String()
}
