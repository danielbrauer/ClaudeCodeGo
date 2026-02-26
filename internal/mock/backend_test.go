package mock

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
)

// testHandler records stream events for assertions.
type testHandler struct {
	textParts []string
	toolNames []string
	stopped   bool
	errCount  int
}

func (h *testHandler) OnMessageStart(_ api.MessageResponse) {}

func (h *testHandler) OnContentBlockStart(_ int, block api.ContentBlock) {
	if block.Type == api.ContentTypeToolUse {
		h.toolNames = append(h.toolNames, block.Name)
	}
}

func (h *testHandler) OnTextDelta(_ int, text string) {
	h.textParts = append(h.textParts, text)
}

func (h *testHandler) OnThinkingDelta(_ int, _ string) {}

func (h *testHandler) OnSignatureDelta(_ int, _ string) {}

func (h *testHandler) OnInputJSONDelta(_ int, _ string) {}

func (h *testHandler) OnContentBlockStop(_ int) {}

func (h *testHandler) OnMessageDelta(_ api.MessageDeltaBody, _ *api.Usage) {}

func (h *testHandler) OnMessageStop() {
	h.stopped = true
}

func (h *testHandler) OnError(_ error) {
	h.errCount++
}

func (h *testHandler) fullText() string {
	var s string
	for _, p := range h.textParts {
		s += p
	}
	return s
}

// --- StaticTokenSource ---

func TestStaticTokenSource(t *testing.T) {
	ts := &StaticTokenSource{Token: "test-token"}
	tok, err := ts.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "test-token" {
		t.Errorf("got %q, want %q", tok, "test-token")
	}
	// InvalidateToken should be a no-op.
	ts.InvalidateToken()
	tok, _ = ts.GetAccessToken(context.Background())
	if tok != "test-token" {
		t.Errorf("after invalidate: got %q, want %q", tok, "test-token")
	}
}

// --- Response builder helpers ---

func TestTextResponse(t *testing.T) {
	resp := TextResponse("hello world", 1)
	if resp.StopReason != api.StopReasonEndTurn {
		t.Errorf("stop_reason = %q, want %q", resp.StopReason, api.StopReasonEndTurn)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content length = %d, want 1", len(resp.Content))
	}
	if resp.Content[0].Text != "hello world" {
		t.Errorf("text = %q, want %q", resp.Content[0].Text, "hello world")
	}
	if resp.Content[0].Type != api.ContentTypeText {
		t.Errorf("type = %q", resp.Content[0].Type)
	}
}

func TestToolUseResponse(t *testing.T) {
	input := json.RawMessage(`{"command":"ls"}`)
	resp := ToolUseResponse("tool_1", "Bash", input, 1)
	if resp.StopReason != api.StopReasonToolUse {
		t.Errorf("stop_reason = %q, want %q", resp.StopReason, api.StopReasonToolUse)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content length = %d, want 1", len(resp.Content))
	}
	block := resp.Content[0]
	if block.Type != api.ContentTypeToolUse {
		t.Errorf("type = %q", block.Type)
	}
	if block.Name != "Bash" {
		t.Errorf("name = %q", block.Name)
	}
	if block.ID != "tool_1" {
		t.Errorf("id = %q", block.ID)
	}
}

func TestToolUseWithTextResponse(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/tmp/x"}`)
	resp := ToolUseWithTextResponse("Let me read that file.", "tool_2", "FileRead", input, 1)
	if len(resp.Content) != 2 {
		t.Fatalf("content length = %d, want 2", len(resp.Content))
	}
	if resp.Content[0].Type != api.ContentTypeText {
		t.Errorf("first block type = %q", resp.Content[0].Type)
	}
	if resp.Content[1].Type != api.ContentTypeToolUse {
		t.Errorf("second block type = %q", resp.Content[1].Type)
	}
}

func TestMultiToolUseResponse(t *testing.T) {
	calls := []ToolCall{
		{ID: "t1", Name: "Glob", Input: json.RawMessage(`{"pattern":"*.go"}`)},
		{ID: "t2", Name: "Grep", Input: json.RawMessage(`{"pattern":"TODO"}`)},
	}
	resp := MultiToolUseResponse(calls, 1)
	if len(resp.Content) != 2 {
		t.Fatalf("content length = %d, want 2", len(resp.Content))
	}
	if resp.Content[0].Name != "Glob" {
		t.Errorf("first tool name = %q", resp.Content[0].Name)
	}
	if resp.Content[1].Name != "Grep" {
		t.Errorf("second tool name = %q", resp.Content[1].Name)
	}
}

// --- Responders ---

func TestStaticResponder(t *testing.T) {
	expected := TextResponse("static", 1)
	r := &StaticResponder{Response: expected}

	got := r.Respond(&api.CreateMessageRequest{})
	if got.Content[0].Text != "static" {
		t.Errorf("text = %q", got.Content[0].Text)
	}

	// Should return the same response every time.
	got2 := r.Respond(&api.CreateMessageRequest{})
	if got2.Content[0].Text != "static" {
		t.Errorf("second call text = %q", got2.Content[0].Text)
	}
}

func TestScriptedResponder(t *testing.T) {
	r := NewScriptedResponder([]*api.MessageResponse{
		TextResponse("first", 1),
		TextResponse("second", 2),
		TextResponse("third", 3),
	})

	// First three calls return the scripted responses.
	if got := r.Respond(&api.CreateMessageRequest{}); got.Content[0].Text != "first" {
		t.Errorf("call 1: %q", got.Content[0].Text)
	}
	if got := r.Respond(&api.CreateMessageRequest{}); got.Content[0].Text != "second" {
		t.Errorf("call 2: %q", got.Content[0].Text)
	}
	if got := r.Respond(&api.CreateMessageRequest{}); got.Content[0].Text != "third" {
		t.Errorf("call 3: %q", got.Content[0].Text)
	}

	// Beyond the script, repeats the last response.
	if got := r.Respond(&api.CreateMessageRequest{}); got.Content[0].Text != "third" {
		t.Errorf("call 4 (overflow): %q", got.Content[0].Text)
	}
}

func TestScriptedResponder_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty responses")
		}
	}()
	NewScriptedResponder(nil)
}

func TestEchoResponder(t *testing.T) {
	r := &EchoResponder{}
	req := &api.CreateMessageRequest{
		Messages: []api.Message{
			api.NewTextMessage(api.RoleUser, "hello there"),
		},
	}
	resp := r.Respond(req)
	if resp.Content[0].Text != "Echo: hello there" {
		t.Errorf("text = %q", resp.Content[0].Text)
	}
	if r.CallCount() != 1 {
		t.Errorf("call count = %d", r.CallCount())
	}
}

func TestResponderFunc(t *testing.T) {
	called := false
	r := ResponderFunc(func(req *api.CreateMessageRequest) *api.MessageResponse {
		called = true
		return TextResponse("func", 1)
	})
	resp := r.Respond(&api.CreateMessageRequest{})
	if !called {
		t.Error("function was not called")
	}
	if resp.Content[0].Text != "func" {
		t.Errorf("text = %q", resp.Content[0].Text)
	}
}

// --- Backend: text response round-trip ---

func TestBackend_TextResponse(t *testing.T) {
	b := NewBackend(&StaticResponder{
		Response: TextResponse("Hello from mock!", 1),
	})
	defer b.Close()

	client := b.Client()
	handler := &testHandler{}

	resp, err := client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "hi")},
	}, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != api.StopReasonEndTurn {
		t.Errorf("stop_reason = %q", resp.StopReason)
	}
	if handler.fullText() != "Hello from mock!" {
		t.Errorf("streamed text = %q", handler.fullText())
	}
	if !handler.stopped {
		t.Error("OnMessageStop was not called")
	}
	if b.RequestCount() != 1 {
		t.Errorf("request count = %d", b.RequestCount())
	}
}

// --- Backend: tool use round-trip ---

func TestBackend_ToolUseResponse(t *testing.T) {
	input := json.RawMessage(`{"command":"echo hello"}`)
	b := NewBackend(&StaticResponder{
		Response: ToolUseResponse("toolu_123", "Bash", input, 1),
	})
	defer b.Close()

	client := b.Client()
	handler := &testHandler{}

	resp, err := client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "run a command")},
	}, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != api.StopReasonToolUse {
		t.Errorf("stop_reason = %q", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content blocks = %d", len(resp.Content))
	}
	if resp.Content[0].Name != "Bash" {
		t.Errorf("tool name = %q", resp.Content[0].Name)
	}
	if string(resp.Content[0].Input) != `{"command":"echo hello"}` {
		t.Errorf("tool input = %s", resp.Content[0].Input)
	}
}

// --- Backend: request capture ---

func TestBackend_CapturesHeaders(t *testing.T) {
	b := NewBackend(&StaticResponder{Response: TextResponse("ok", 1)})
	defer b.Close()

	client := b.Client(api.WithVersion("9.9.9"))
	client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "hi")},
	}, &testHandler{})

	req := b.LastRequest()
	if req == nil {
		t.Fatal("no request captured")
	}
	if req.Headers.Get("Authorization") != "Bearer mock-token" {
		t.Errorf("auth header = %q", req.Headers.Get("Authorization"))
	}
	if req.Headers.Get("User-Agent") != "claude-code/9.9.9" {
		t.Errorf("user-agent = %q", req.Headers.Get("User-Agent"))
	}
}

func TestBackend_CapturesRequestBody(t *testing.T) {
	b := NewBackend(&StaticResponder{Response: TextResponse("ok", 1)})
	defer b.Close()

	client := b.Client()
	client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "what is 2+2?")},
	}, &testHandler{})

	req := b.LastRequest()
	if req == nil {
		t.Fatal("no request captured")
	}
	if req.Body == nil {
		t.Fatal("body not parsed")
	}
	if len(req.Body.Messages) != 1 {
		t.Errorf("messages count = %d", len(req.Body.Messages))
	}
}

// --- Backend: echo responder ---

func TestBackend_EchoResponder(t *testing.T) {
	b := NewBackend(&EchoResponder{})
	defer b.Close()

	client := b.Client()
	handler := &testHandler{}

	resp, err := client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "ping")},
	}, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.fullText() != "Echo: ping" {
		t.Errorf("streamed text = %q", handler.fullText())
	}
	if resp.StopReason != api.StopReasonEndTurn {
		t.Errorf("stop_reason = %q", resp.StopReason)
	}
}

// --- Backend: SetResponder ---

func TestBackend_SetResponder(t *testing.T) {
	b := NewBackend(&StaticResponder{Response: TextResponse("first", 1)})
	defer b.Close()

	client := b.Client()

	// First call gets "first".
	h1 := &testHandler{}
	client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "hi")},
	}, h1)
	if h1.fullText() != "first" {
		t.Errorf("first response = %q", h1.fullText())
	}

	// Swap the responder.
	b.SetResponder(&StaticResponder{Response: TextResponse("second", 2)})

	// Second call gets "second".
	h2 := &testHandler{}
	client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "hi again")},
	}, h2)
	if h2.fullText() != "second" {
		t.Errorf("second response = %q", h2.fullText())
	}
}

// --- Backend: scripted multi-turn ---

func TestBackend_ScriptedMultiTurn(t *testing.T) {
	r := NewScriptedResponder([]*api.MessageResponse{
		TextResponse("response 1", 1),
		TextResponse("response 2", 2),
	})
	b := NewBackend(r)
	defer b.Close()

	client := b.Client()

	// Turn 1.
	h1 := &testHandler{}
	client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "first")},
	}, h1)
	if h1.fullText() != "response 1" {
		t.Errorf("turn 1: %q", h1.fullText())
	}

	// Turn 2.
	h2 := &testHandler{}
	client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{
			api.NewTextMessage(api.RoleUser, "first"),
			api.NewTextMessage(api.RoleAssistant, "response 1"),
			api.NewTextMessage(api.RoleUser, "second"),
		},
	}, h2)
	if h2.fullText() != "response 2" {
		t.Errorf("turn 2: %q", h2.fullText())
	}

	if b.RequestCount() != 2 {
		t.Errorf("total requests = %d", b.RequestCount())
	}
}

// --- Backend: mixed text + tool_use blocks ---

func TestBackend_TextAndToolUseBlocks(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/tmp/test.go"}`)
	b := NewBackend(&StaticResponder{
		Response: ToolUseWithTextResponse(
			"Let me read the file.", "toolu_abc", "FileRead", input, 1,
		),
	})
	defer b.Close()

	client := b.Client()
	handler := &testHandler{}

	resp, err := client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "read /tmp/test.go")},
	}, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.fullText() != "Let me read the file." {
		t.Errorf("text = %q", handler.fullText())
	}
	if len(handler.toolNames) != 1 || handler.toolNames[0] != "FileRead" {
		t.Errorf("tool names = %v", handler.toolNames)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("content blocks = %d", len(resp.Content))
	}
}

// --- Backend: multi tool use ---

func TestBackend_MultiToolUse(t *testing.T) {
	calls := []ToolCall{
		{ID: "t1", Name: "Glob", Input: json.RawMessage(`{"pattern":"*.go"}`)},
		{ID: "t2", Name: "Grep", Input: json.RawMessage(`{"pattern":"func main"}`)},
	}
	b := NewBackend(&StaticResponder{
		Response: MultiToolUseResponse(calls, 1),
	})
	defer b.Close()

	client := b.Client()
	handler := &testHandler{}

	resp, err := client.CreateMessageStream(context.Background(), &api.CreateMessageRequest{
		Messages: []api.Message{api.NewTextMessage(api.RoleUser, "find go files")},
	}, handler)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != api.StopReasonToolUse {
		t.Errorf("stop_reason = %q", resp.StopReason)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("content blocks = %d", len(resp.Content))
	}
	if resp.Content[0].Name != "Glob" {
		t.Errorf("block 0 name = %q", resp.Content[0].Name)
	}
	if resp.Content[1].Name != "Grep" {
		t.Errorf("block 1 name = %q", resp.Content[1].Name)
	}
}
