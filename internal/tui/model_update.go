package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/tools"
)

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textarea.Blink,
	}
	// If there's an initial prompt, send it immediately.
	if m.initialPrompt != "" {
		cmds = append(cmds, func() tea.Msg {
			return SubmitInputMsg{Text: m.initialPrompt}
		})
	}
	// Kick off the initial status line refresh.
	if cmd := m.refreshStatusLine(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// ── Terminal resize ──
	case tea.WindowSizeMsg:
		// Guard against zero/negative dimensions that can arrive from
		// pseudo-terminals or piped output; keep the previous values.
		if msg.Width > 0 {
			m.width = msg.Width
			m.textInput.SetWidth(msg.Width)
			m.mdRenderer.updateWidth(msg.Width)
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m, nil

	// ── Key events ──
	case tea.KeyMsg:
		return m.handleKey(msg)

	// ── User submits input ──
	case SubmitInputMsg:
		return m.handleSubmit(msg.Text)

	// ── Stream handler messages ──
	case MessageStartMsg:
		m.tokens.addInput(msg.Usage.InputTokens,
			msg.Usage.CacheReadInputTokens, msg.Usage.CacheCreationInputTokens)
		if msg.Model != "" {
			m.resolvedModelID = msg.Model
		}
		return m, nil

	case ContentBlockStartMsg:
		if msg.Block.Type == api.ContentTypeToolUse {
			m.activeTool = msg.Block.Name
			m.toolSummary = ""
			return m, m.spinner.Tick
		}
		return m, nil

	case TextDeltaMsg:
		m.streamingText += msg.Text
		return m, nil

	case InputJSONDeltaMsg:
		// We don't need to track JSON here (stream handler does it).
		return m, nil

	case ContentBlockStopMsg:
		if msg.Name != "" {
			// Tool call block completed. Print the tool line to scrollback.
			toolLine := renderToolComplete(msg.Name, msg.Input)
			cmds = append(cmds, tea.Println(toolLine))
			m.activeTool = ""
			m.toolSummary = ""
		} else if m.streamingText != "" {
			// Text block completed. Flush to scrollback.
			rendered := m.mdRenderer.render(m.streamingText)
			cmds = append(cmds, tea.Println(rendered))
			m.streamingText = ""
		}
		return m, tea.Batch(cmds...)

	case MessageDeltaMsg:
		if msg.Usage != nil {
			m.tokens.addOutput(msg.Usage.OutputTokens)
		}
		return m, nil

	case MessageStopMsg:
		// Message finished but the loop may continue (tool results → next API call).
		// Don't switch to input mode here; wait for LoopDoneMsg.
		return m, nil

	case StreamErrorMsg:
		errLine := errorStyle.Render("Error: " + msg.Err.Error())
		cmds = append(cmds, tea.Println(errLine))
		return m, tea.Batch(cmds...)

	case LoopDoneMsg:
		return m.handleLoopDone(msg)

	case promptSuggestionResult:
		m.dynSuggestionGenerating = false
		m.dynSuggestion = msg.text
		return m, nil

	// ── Memory edit done ──
	case MemoryEditDoneMsg:
		var output string
		if msg.Err != nil {
			output = "Editor exited with error: " + msg.Err.Error()
		} else {
			output = editorHintMessage(msg.Path)
		}
		m.mode = modeInput
		m.textInput.Focus()
		return m, tea.Batch(tea.Println(output), textarea.Blink)

	// ── Permission prompt ──
	case PermissionRequestMsg:
		m.permissionPending = &msg
		m.mode = modePermission
		return m, nil

	// ── Status line update ──
	case statusLineUpdateMsg:
		m.statusLineText = msg.Text
		return m, nil

	// ── Todo list update ──
	case tools.TodoUpdateMsg:
		m.todos = msg.Todos
		return m, nil

	// ── AskUser request ──
	case tools.AskUserRequestMsg:
		m.askUserPending = &msg
		m.askCursor = 0
		m.askAnswers = make(map[string]string)
		m.askQuestionIdx = 0
		m.askCustomInput = false
		m.askCustomText = ""
		m.mode = modeAskUser
		return m, nil

	// ── Diff dialog loaded ──
	case DiffLoadedMsg:
		m.diffData = &msg.Data
		m.diffSelected = 0
		m.diffViewMode = "list"
		m.mode = modeDiff
		return m, nil

	// ── Ctrl-C double-press timeout ──
	case ctrlCResetMsg:
		m.ctrlCPending = false
		return m, nil

	// ── Spinner tick ──
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Pass other messages (cursor blink, etc.) to the text input.
	// Only forward to the textarea — don't manipulate height here since
	// these messages don't change content. Height adjustment is only
	// needed after key input (handled in handleInputKey/handleStreamingKey).
	if m.mode == modeInput || m.mode == modeStreaming {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleLoopDone processes the LoopDoneMsg when the agentic loop finishes.
func (m model) handleLoopDone(msg LoopDoneMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Flush any remaining streaming text.
	if m.streamingText != "" {
		rendered := m.mdRenderer.render(m.streamingText)
		cmds = append(cmds, tea.Println(rendered))
		m.streamingText = ""
	}
	if msg.Err != nil && m.ctx.Err() == nil {
		errLine := errorStyle.Render("Error: " + msg.Err.Error())
		cmds = append(cmds, tea.Println(errLine))
	}
	m.activeTool = ""
	// Clear any previous dynamic suggestion.
	m.dynSuggestion = ""
	// Refresh the custom status line after each assistant turn.
	if cmd := m.refreshStatusLine(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	// If there are queued messages, automatically send the next one
	// instead of returning to input mode.
	if text, ok := m.queue.Dequeue(); ok {
		// Stay in streaming mode and submit the queued message.
		return m.handleSubmit(text)
	}

	m.mode = modeInput
	m.textInput.Focus()
	// Generate a dynamic prompt suggestion for the next turn.
	if m.apiClient != nil && msg.Err == nil && m.ctx.Err() == nil {
		m.dynSuggestionGenerating = true
		msgs := m.loop.History().Messages()
		// Copy messages to avoid races with the main loop.
		msgsCopy := make([]api.Message, len(msgs))
		copy(msgsCopy, msgs)
		cmds = append(cmds, generatePromptSuggestionCmd(m.ctx, m.apiClient, msgsCopy))
	}
	return m, tea.Batch(append(cmds, textarea.Blink)...)
}
