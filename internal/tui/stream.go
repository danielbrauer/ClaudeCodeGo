package tui

import (
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
)

// TUIStreamHandler implements api.StreamHandler by forwarding all events
// to the Bubble Tea program via Send(). This bridges the blocking agentic
// loop goroutine with the BT event loop.
type TUIStreamHandler struct {
	program *tea.Program

	// Tool call assembly state (same as ToolAwareStreamHandler).
	toolNames map[int]string
	jsonBufs  map[int][]byte
}

// NewTUIStreamHandler creates a stream handler wired to the given BT program.
func NewTUIStreamHandler(p *tea.Program) *TUIStreamHandler {
	return &TUIStreamHandler{program: p}
}

func (h *TUIStreamHandler) OnMessageStart(msg api.MessageResponse) {
	h.program.Send(MessageStartMsg{Usage: msg.Usage})
}

func (h *TUIStreamHandler) OnContentBlockStart(index int, block api.ContentBlock) {
	if block.Type == api.ContentTypeToolUse {
		if h.toolNames == nil {
			h.toolNames = make(map[int]string)
			h.jsonBufs = make(map[int][]byte)
		}
		h.toolNames[index] = block.Name
		h.jsonBufs[index] = nil
	}
	h.program.Send(ContentBlockStartMsg{Index: index, Block: block})
}

func (h *TUIStreamHandler) OnTextDelta(index int, text string) {
	h.program.Send(TextDeltaMsg{Index: index, Text: text})
}

func (h *TUIStreamHandler) OnThinkingDelta(index int, thinking string) {}

func (h *TUIStreamHandler) OnSignatureDelta(index int, signature string) {}

func (h *TUIStreamHandler) OnInputJSONDelta(index int, partialJSON string) {
	if h.jsonBufs != nil {
		h.jsonBufs[index] = append(h.jsonBufs[index], []byte(partialJSON)...)
	}
	h.program.Send(InputJSONDeltaMsg{Index: index, JSON: partialJSON})
}

func (h *TUIStreamHandler) OnContentBlockStop(index int) {
	name := ""
	var input json.RawMessage
	if n, ok := h.toolNames[index]; ok {
		name = n
		input = json.RawMessage(h.jsonBufs[index])
		delete(h.toolNames, index)
		delete(h.jsonBufs, index)
	}
	h.program.Send(ContentBlockStopMsg{Index: index, Name: name, Input: input})
}

func (h *TUIStreamHandler) OnMessageDelta(delta api.MessageDeltaBody, usage *api.Usage) {
	h.program.Send(MessageDeltaMsg{Delta: delta, Usage: usage})
}

func (h *TUIStreamHandler) OnMessageStop() {
	h.program.Send(MessageStopMsg{})
}

func (h *TUIStreamHandler) OnError(err error) {
	h.program.Send(StreamErrorMsg{Err: err})
}
