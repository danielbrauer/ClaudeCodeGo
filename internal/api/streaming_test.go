package api

import (
	"strings"
	"testing"
)

// testHandler records events for test assertions.
type testHandler struct {
	messageStarts      int
	contentBlockStarts []ContentBlock
	textDeltas         []string
	jsonDeltas         []string
	contentBlockStops  []int
	messageDelta       *MessageDeltaBody
	messageStops       int
	errors             []error
}

func (h *testHandler) OnMessageStart(msg MessageResponse) {
	h.messageStarts++
}

func (h *testHandler) OnContentBlockStart(index int, block ContentBlock) {
	h.contentBlockStarts = append(h.contentBlockStarts, block)
}

func (h *testHandler) OnTextDelta(index int, text string) {
	h.textDeltas = append(h.textDeltas, text)
}

func (h *testHandler) OnThinkingDelta(index int, thinking string) {}

func (h *testHandler) OnSignatureDelta(index int, signature string) {}

func (h *testHandler) OnInputJSONDelta(index int, partialJSON string) {
	h.jsonDeltas = append(h.jsonDeltas, partialJSON)
}

func (h *testHandler) OnContentBlockStop(index int) {
	h.contentBlockStops = append(h.contentBlockStops, index)
}

func (h *testHandler) OnMessageDelta(delta MessageDeltaBody, usage *Usage) {
	h.messageDelta = &delta
}

func (h *testHandler) OnMessageStop() {
	h.messageStops++
}

func (h *testHandler) OnError(err error) {
	h.errors = append(h.errors, err)
}

func TestParseSSEStream_TextResponse(t *testing.T) {
	stream := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}

`

	h := &testHandler{}
	err := ParseSSEStream(strings.NewReader(stream), h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.messageStarts != 1 {
		t.Errorf("expected 1 message_start, got %d", h.messageStarts)
	}

	if len(h.contentBlockStarts) != 1 {
		t.Errorf("expected 1 content_block_start, got %d", len(h.contentBlockStarts))
	}

	if len(h.textDeltas) != 2 {
		t.Errorf("expected 2 text deltas, got %d", len(h.textDeltas))
	}
	if h.textDeltas[0] != "Hello" || h.textDeltas[1] != " world" {
		t.Errorf("unexpected text deltas: %v", h.textDeltas)
	}

	if len(h.contentBlockStops) != 1 || h.contentBlockStops[0] != 0 {
		t.Errorf("unexpected content_block_stops: %v", h.contentBlockStops)
	}

	if h.messageDelta == nil || h.messageDelta.StopReason != "end_turn" {
		t.Errorf("expected stop_reason=end_turn, got %+v", h.messageDelta)
	}

	if h.messageStops != 1 {
		t.Errorf("expected 1 message_stop, got %d", h.messageStops)
	}

	if len(h.errors) != 0 {
		t.Errorf("unexpected errors: %v", h.errors)
	}
}

func TestParseSSEStream_ToolUse(t *testing.T) {
	stream := `event: message_start
data: {"type":"message_start","message":{"id":"msg_02","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"Bash","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"comma"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"nd\":\"ls\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}

event: message_stop
data: {"type":"message_stop"}

`

	h := &testHandler{}
	err := ParseSSEStream(strings.NewReader(stream), h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(h.contentBlockStarts) != 1 {
		t.Fatalf("expected 1 content_block_start, got %d", len(h.contentBlockStarts))
	}
	if h.contentBlockStarts[0].Name != "Bash" {
		t.Errorf("expected tool name Bash, got %s", h.contentBlockStarts[0].Name)
	}

	if len(h.jsonDeltas) != 2 {
		t.Errorf("expected 2 json deltas, got %d", len(h.jsonDeltas))
	}

	fullJSON := strings.Join(h.jsonDeltas, "")
	if fullJSON != `{"command":"ls"}` {
		t.Errorf("unexpected assembled JSON: %s", fullJSON)
	}

	if h.messageDelta == nil || h.messageDelta.StopReason != "tool_use" {
		t.Errorf("expected stop_reason=tool_use, got %+v", h.messageDelta)
	}
}

func TestParseSSEStream_Ping(t *testing.T) {
	stream := `event: ping
data: {"type":"ping"}

`
	h := &testHandler{}
	err := ParseSSEStream(strings.NewReader(stream), h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.messageStarts != 0 {
		t.Errorf("expected no events dispatched for ping, got messageStarts=%d", h.messageStarts)
	}
}
