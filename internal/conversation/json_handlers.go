package conversation

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/anthropics/claude-code-go/internal/api"
)

// JSONStreamHandler collects the full response and emits a single JSON object
// when the message is complete. Used with --output-format json.
type JSONStreamHandler struct {
	writer     io.Writer
	content    []api.ContentBlock
	usage      api.Usage
	model      string
	stopReason string

	// Tool call assembly state.
	toolNames map[int]string
	jsonBufs  map[int][]byte
	textBufs  map[int]string
}

// NewJSONStreamHandler creates a handler that writes a single JSON message.
func NewJSONStreamHandler(w io.Writer) *JSONStreamHandler {
	return &JSONStreamHandler{
		writer:    w,
		toolNames: make(map[int]string),
		jsonBufs:  make(map[int][]byte),
		textBufs:  make(map[int]string),
	}
}

func (h *JSONStreamHandler) OnMessageStart(msg api.MessageResponse) {
	h.model = msg.Model
	h.usage.InputTokens = msg.Usage.InputTokens
	if msg.Usage.CacheReadInputTokens != nil {
		h.usage.CacheReadInputTokens = msg.Usage.CacheReadInputTokens
	}
	if msg.Usage.CacheCreationInputTokens != nil {
		h.usage.CacheCreationInputTokens = msg.Usage.CacheCreationInputTokens
	}
}

func (h *JSONStreamHandler) OnContentBlockStart(index int, block api.ContentBlock) {
	if block.Type == api.ContentTypeToolUse {
		h.toolNames[index] = block.Name
		h.jsonBufs[index] = nil
	}
}

func (h *JSONStreamHandler) OnTextDelta(index int, text string) {
	h.textBufs[index] += text
}

func (h *JSONStreamHandler) OnThinkingDelta(index int, thinking string) {}

func (h *JSONStreamHandler) OnSignatureDelta(index int, signature string) {}

func (h *JSONStreamHandler) OnInputJSONDelta(index int, partialJSON string) {
	h.jsonBufs[index] = append(h.jsonBufs[index], []byte(partialJSON)...)
}

func (h *JSONStreamHandler) OnContentBlockStop(index int) {
	if name, ok := h.toolNames[index]; ok {
		block := api.ContentBlock{
			Type:  api.ContentTypeToolUse,
			Name:  name,
			Input: json.RawMessage(h.jsonBufs[index]),
		}
		h.content = append(h.content, block)
		delete(h.toolNames, index)
		delete(h.jsonBufs, index)
	} else if text, ok := h.textBufs[index]; ok && text != "" {
		block := api.ContentBlock{
			Type: api.ContentTypeText,
			Text: text,
		}
		h.content = append(h.content, block)
		delete(h.textBufs, index)
	}
}

func (h *JSONStreamHandler) OnMessageDelta(delta api.MessageDeltaBody, usage *api.Usage) {
	if delta.StopReason != "" {
		h.stopReason = delta.StopReason
	}
	if usage != nil {
		h.usage.OutputTokens = usage.OutputTokens
	}
}

func (h *JSONStreamHandler) OnMessageStop() {
	msg := map[string]interface{}{
		"type":        "message",
		"role":        "assistant",
		"content":     h.content,
		"model":       h.model,
		"stop_reason": h.stopReason,
		"usage": map[string]interface{}{
			"input_tokens":  h.usage.InputTokens,
			"output_tokens": h.usage.OutputTokens,
		},
	}
	data, _ := json.Marshal(msg)
	fmt.Fprintln(h.writer, string(data))
}

func (h *JSONStreamHandler) OnError(err error) {
	errMsg := map[string]interface{}{
		"type":  "error",
		"error": err.Error(),
	}
	data, _ := json.Marshal(errMsg)
	fmt.Fprintln(h.writer, string(data))
}

// StreamJSONStreamHandler emits one JSON line per streaming event as it
// arrives. Used with --output-format stream-json.
type StreamJSONStreamHandler struct {
	writer io.Writer
}

// NewStreamJSONStreamHandler creates a handler that writes one JSON line per event.
func NewStreamJSONStreamHandler(w io.Writer) *StreamJSONStreamHandler {
	return &StreamJSONStreamHandler{writer: w}
}

func (h *StreamJSONStreamHandler) emit(v interface{}) {
	data, _ := json.Marshal(v)
	fmt.Fprintln(h.writer, string(data))
}

func (h *StreamJSONStreamHandler) OnMessageStart(msg api.MessageResponse) {
	h.emit(map[string]interface{}{
		"type":    "message_start",
		"message": msg,
	})
}

func (h *StreamJSONStreamHandler) OnContentBlockStart(index int, block api.ContentBlock) {
	h.emit(map[string]interface{}{
		"type":          "content_block_start",
		"index":         index,
		"content_block": block,
	})
}

func (h *StreamJSONStreamHandler) OnTextDelta(index int, text string) {
	h.emit(map[string]interface{}{
		"type":  "text_delta",
		"index": index,
		"text":  text,
	})
}

func (h *StreamJSONStreamHandler) OnThinkingDelta(index int, thinking string) {
	h.emit(map[string]interface{}{
		"type":     "thinking_delta",
		"index":    index,
		"thinking": thinking,
	})
}

func (h *StreamJSONStreamHandler) OnSignatureDelta(index int, signature string) {}

func (h *StreamJSONStreamHandler) OnInputJSONDelta(index int, partialJSON string) {
	h.emit(map[string]interface{}{
		"type":         "input_json_delta",
		"index":        index,
		"partial_json": partialJSON,
	})
}

func (h *StreamJSONStreamHandler) OnContentBlockStop(index int) {
	h.emit(map[string]interface{}{
		"type":  "content_block_stop",
		"index": index,
	})
}

func (h *StreamJSONStreamHandler) OnMessageDelta(delta api.MessageDeltaBody, usage *api.Usage) {
	evt := map[string]interface{}{
		"type":  "message_delta",
		"delta": delta,
	}
	if usage != nil {
		evt["usage"] = usage
	}
	h.emit(evt)
}

func (h *StreamJSONStreamHandler) OnMessageStop() {
	h.emit(map[string]interface{}{
		"type": "message_stop",
	})
}

func (h *StreamJSONStreamHandler) OnError(err error) {
	h.emit(map[string]interface{}{
		"type":  "error",
		"error": err.Error(),
	})
}
