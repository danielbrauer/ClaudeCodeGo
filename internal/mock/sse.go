package mock

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/anthropics/claude-code-go/internal/api"
)

// WriteSSEResponse writes a complete MessageResponse as a properly formatted
// SSE stream to the writer. This generates the same event sequence the real
// API produces: message_start, content_block_start/delta/stop for each block,
// message_delta, message_stop.
func WriteSSEResponse(w io.Writer, resp *api.MessageResponse) error {
	// message_start — contains the message shell (no content yet).
	msgShell := api.MessageResponse{
		ID:    resp.ID,
		Type:  resp.Type,
		Role:  resp.Role,
		Model: resp.Model,
		Usage: api.Usage{InputTokens: resp.Usage.InputTokens},
	}
	if err := writeSSEEvent(w, api.EventMessageStart, api.MessageStartData{
		Type:    api.EventMessageStart,
		Message: msgShell,
	}); err != nil {
		return err
	}

	// Each content block: start, deltas, stop.
	for i, block := range resp.Content {
		switch block.Type {
		case api.ContentTypeText:
			if err := writeTextBlock(w, i, block); err != nil {
				return err
			}
		case api.ContentTypeToolUse:
			if err := writeToolUseBlock(w, i, block); err != nil {
				return err
			}
		}
	}

	// message_delta — carries stop_reason and final usage.
	if err := writeSSEEvent(w, api.EventMessageDelta, api.MessageDeltaData{
		Type: api.EventMessageDelta,
		Delta: api.MessageDeltaBody{
			StopReason:   resp.StopReason,
			StopSequence: resp.StopSequence,
		},
		Usage: &api.Usage{OutputTokens: resp.Usage.OutputTokens},
	}); err != nil {
		return err
	}

	// message_stop.
	return writeSSEEvent(w, api.EventMessageStop, struct {
		Type string `json:"type"`
	}{Type: api.EventMessageStop})
}

func writeTextBlock(w io.Writer, index int, block api.ContentBlock) error {
	// content_block_start with empty text.
	if err := writeSSEEvent(w, api.EventContentBlockStart, api.ContentBlockStartData{
		Type:  api.EventContentBlockStart,
		Index: index,
		ContentBlock: api.ContentBlock{
			Type: api.ContentTypeText,
			Text: "",
		},
	}); err != nil {
		return err
	}

	// Send text in chunks to simulate streaming.
	text := block.Text
	for len(text) > 0 {
		chunk := text
		if len(chunk) > 50 {
			chunk = text[:50]
		}
		text = text[len(chunk):]

		if err := writeSSEEvent(w, api.EventContentBlockDelta, api.ContentBlockDeltaData{
			Type:  api.EventContentBlockDelta,
			Index: index,
			Delta: api.BlockDelta{
				Type: "text_delta",
				Text: chunk,
			},
		}); err != nil {
			return err
		}
	}

	// content_block_stop.
	return writeSSEEvent(w, api.EventContentBlockStop, api.ContentBlockStopData{
		Type:  api.EventContentBlockStop,
		Index: index,
	})
}

func writeToolUseBlock(w io.Writer, index int, block api.ContentBlock) error {
	// content_block_start with tool name and ID, empty input.
	if err := writeSSEEvent(w, api.EventContentBlockStart, api.ContentBlockStartData{
		Type:  api.EventContentBlockStart,
		Index: index,
		ContentBlock: api.ContentBlock{
			Type: api.ContentTypeToolUse,
			ID:   block.ID,
			Name: block.Name,
		},
	}); err != nil {
		return err
	}

	// Send input JSON as deltas.
	inputJSON := string(block.Input)
	for len(inputJSON) > 0 {
		chunk := inputJSON
		if len(chunk) > 80 {
			chunk = inputJSON[:80]
		}
		inputJSON = inputJSON[len(chunk):]

		if err := writeSSEEvent(w, api.EventContentBlockDelta, api.ContentBlockDeltaData{
			Type:  api.EventContentBlockDelta,
			Index: index,
			Delta: api.BlockDelta{
				Type:        "input_json_delta",
				PartialJSON: chunk,
			},
		}); err != nil {
			return err
		}
	}

	// content_block_stop.
	return writeSSEEvent(w, api.EventContentBlockStop, api.ContentBlockStopData{
		Type:  api.EventContentBlockStop,
		Index: index,
	})
}

func writeSSEEvent(w io.Writer, eventType string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling SSE data for %s: %w", eventType, err)
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, jsonData)
	return err
}
