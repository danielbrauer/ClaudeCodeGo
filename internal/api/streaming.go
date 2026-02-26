package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// SSE event type constants.
const (
	EventMessageStart      = "message_start"
	EventContentBlockStart = "content_block_start"
	EventContentBlockDelta = "content_block_delta"
	EventContentBlockStop  = "content_block_stop"
	EventMessageDelta      = "message_delta"
	EventMessageStop       = "message_stop"
	EventPing              = "ping"
	EventError             = "error"
)

// StreamEvent is the parsed representation of an SSE event from the Messages API.
type StreamEvent struct {
	Type string
	Data json.RawMessage
}

// MessageStartData is the data for a message_start event.
type MessageStartData struct {
	Type    string          `json:"type"`
	Message MessageResponse `json:"message"`
}

// ContentBlockStartData is the data for a content_block_start event.
type ContentBlockStartData struct {
	Type         string       `json:"type"`
	Index        int          `json:"index"`
	ContentBlock ContentBlock `json:"content_block"`
}

// ContentBlockDeltaData is the data for a content_block_delta event.
type ContentBlockDeltaData struct {
	Type  string     `json:"type"`
	Index int        `json:"index"`
	Delta BlockDelta `json:"delta"`
}

// BlockDelta represents the incremental update in a content_block_delta event.
type BlockDelta struct {
	Type        string `json:"type"`                   // "text_delta", "input_json_delta", "thinking_delta", or "signature_delta"
	Text        string `json:"text,omitempty"`          // for text_delta
	PartialJSON string `json:"partial_json,omitempty"`  // for input_json_delta
	Thinking    string `json:"thinking,omitempty"`      // for thinking_delta
	Signature   string `json:"signature,omitempty"`     // for signature_delta
}

// ContentBlockStopData is the data for a content_block_stop event.
type ContentBlockStopData struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// MessageDeltaData is the data for a message_delta event.
type MessageDeltaData struct {
	Type  string           `json:"type"`
	Delta MessageDeltaBody `json:"delta"`
	Usage *Usage           `json:"usage,omitempty"`
}

// MessageDeltaBody contains the delta fields in a message_delta event.
type MessageDeltaBody struct {
	StopReason   string  `json:"stop_reason,omitempty"`
	StopSequence *string `json:"stop_sequence,omitempty"`
}

// StreamHandler processes streaming events from the Messages API.
// Callers implement this interface to handle events as they arrive.
type StreamHandler interface {
	OnMessageStart(msg MessageResponse)
	OnContentBlockStart(index int, block ContentBlock)
	OnTextDelta(index int, text string)
	OnThinkingDelta(index int, thinking string)
	OnSignatureDelta(index int, signature string)
	OnInputJSONDelta(index int, partialJSON string)
	OnContentBlockStop(index int)
	OnMessageDelta(delta MessageDeltaBody, usage *Usage)
	OnMessageStop()
	OnError(err error)
}

// ParseSSEStream reads an SSE stream from the reader and dispatches events
// to the handler. It blocks until the stream ends or an error occurs.
func ParseSSEStream(r io.Reader, handler StreamHandler) error {
	scanner := bufio.NewScanner(r)
	// SSE can have lines up to several MB for large tool call JSON.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var eventType string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Empty line = end of event. Dispatch if we have data.
			if eventType != "" && len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				if err := dispatchEvent(eventType, []byte(data), handler); err != nil {
					handler.OnError(fmt.Errorf("dispatching event %s: %w", eventType, err))
				}
			}
			eventType = ""
			dataLines = nil
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		} else if line == "data:" {
			dataLines = append(dataLines, "")
		}
		// Ignore comments (lines starting with ':') and other fields.
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading SSE stream: %w", err)
	}
	return nil
}

func dispatchEvent(eventType string, data []byte, handler StreamHandler) error {
	switch eventType {
	case EventMessageStart:
		var d MessageStartData
		if err := json.Unmarshal(data, &d); err != nil {
			return err
		}
		handler.OnMessageStart(d.Message)

	case EventContentBlockStart:
		var d ContentBlockStartData
		if err := json.Unmarshal(data, &d); err != nil {
			return err
		}
		handler.OnContentBlockStart(d.Index, d.ContentBlock)

	case EventContentBlockDelta:
		var d ContentBlockDeltaData
		if err := json.Unmarshal(data, &d); err != nil {
			return err
		}
		switch d.Delta.Type {
		case "text_delta":
			handler.OnTextDelta(d.Index, d.Delta.Text)
		case "thinking_delta":
			handler.OnThinkingDelta(d.Index, d.Delta.Thinking)
		case "signature_delta":
			handler.OnSignatureDelta(d.Index, d.Delta.Signature)
		case "input_json_delta":
			handler.OnInputJSONDelta(d.Index, d.Delta.PartialJSON)
		}

	case EventContentBlockStop:
		var d ContentBlockStopData
		if err := json.Unmarshal(data, &d); err != nil {
			return err
		}
		handler.OnContentBlockStop(d.Index)

	case EventMessageDelta:
		var d MessageDeltaData
		if err := json.Unmarshal(data, &d); err != nil {
			return err
		}
		handler.OnMessageDelta(d.Delta, d.Usage)

	case EventMessageStop:
		handler.OnMessageStop()

	case EventPing:
		// Ignore keepalive pings.

	case EventError:
		var apiErr APIError
		if err := json.Unmarshal(data, &apiErr); err != nil {
			return fmt.Errorf("API error (unparseable): %s", string(data))
		}
		handler.OnError(fmt.Errorf("API error: %s: %s", apiErr.Error.Type, apiErr.Error.Message))

	default:
		// Unknown event types are ignored per SSE spec.
	}
	return nil
}
