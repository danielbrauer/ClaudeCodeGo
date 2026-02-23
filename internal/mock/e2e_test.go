package mock_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/mock"
	"github.com/anthropics/claude-code-go/internal/tools"
)

// collectingHandler records all streamed text from the assistant.
type collectingHandler struct {
	texts []string
}

func (h *collectingHandler) OnMessageStart(_ api.MessageResponse)              {}
func (h *collectingHandler) OnContentBlockStart(_ int, _ api.ContentBlock)     {}
func (h *collectingHandler) OnTextDelta(_ int, text string)                    { h.texts = append(h.texts, text) }
func (h *collectingHandler) OnInputJSONDelta(_ int, _ string)                  {}
func (h *collectingHandler) OnContentBlockStop(_ int)                          {}
func (h *collectingHandler) OnMessageDelta(_ api.MessageDeltaBody, _ *api.Usage) {}
func (h *collectingHandler) OnMessageStop()                                    {}
func (h *collectingHandler) OnError(_ error)                                   {}

func (h *collectingHandler) fullText() string {
	return strings.Join(h.texts, "")
}

// setupLoop creates a mock backend, API client, tool registry, and
// conversation loop wired together for end-to-end testing.
func setupLoop(t *testing.T, responder mock.Responder, handler api.StreamHandler) (*mock.Backend, *conversation.Loop) {
	t.Helper()

	b := mock.NewBackend(responder)
	t.Cleanup(b.Close)

	client := b.Client()

	// Use a temp dir as workdir for tools.
	workDir := t.TempDir()

	// Set up a registry with real tools that don't need permissions.
	registry := tools.NewRegistry(&tools.AlwaysAllowPermissionHandler{})
	registry.Register(tools.NewFileReadTool())
	registry.Register(tools.NewFileWriteTool())
	registry.Register(tools.NewGlobTool(workDir))
	registry.Register(tools.NewGrepTool(workDir))

	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:   client,
		Tools:    registry.Definitions(),
		ToolExec: registry,
		Handler:  handler,
	})

	return b, loop
}

// --- E2E: simple text conversation ---

func TestE2E_SimpleTextResponse(t *testing.T) {
	handler := &collectingHandler{}
	_, loop := setupLoop(t, &mock.StaticResponder{
		Response: mock.TextResponse("Hello! How can I help you today?", 1),
	}, handler)

	err := loop.SendMessage(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	got := handler.fullText()
	if got != "Hello! How can I help you today?" {
		t.Errorf("response text = %q", got)
	}

	// History should have 2 messages: user + assistant.
	if loop.History().Len() != 2 {
		t.Errorf("history length = %d, want 2", loop.History().Len())
	}
}

// --- E2E: tool use (FileWrite) then final response ---

func TestE2E_ToolUseThenResponse(t *testing.T) {
	workDir := t.TempDir()
	filePath := filepath.Join(workDir, "output.txt")

	// The mock will first request a FileWrite, then return a final text.
	writeInput, _ := json.Marshal(map[string]interface{}{
		"file_path": filePath,
		"content":   "Hello from mock!",
	})

	responder := mock.NewScriptedResponder([]*api.MessageResponse{
		// Turn 1: model requests FileWrite tool.
		mock.ToolUseResponse("toolu_write_1", "FileWrite", writeInput, 1),
		// Turn 2: model returns final text (after seeing tool result).
		mock.TextResponse("I've written the file for you.", 2),
	})

	handler := &collectingHandler{}

	b := mock.NewBackend(responder)
	t.Cleanup(b.Close)

	client := b.Client()

	registry := tools.NewRegistry(&tools.AlwaysAllowPermissionHandler{})
	registry.Register(tools.NewFileWriteTool())

	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:   client,
		Tools:    registry.Definitions(),
		ToolExec: registry,
		Handler:  handler,
	})

	err := loop.SendMessage(context.Background(), "Write 'Hello from mock!' to output.txt")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Verify the file was actually written by the tool.
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(content) != "Hello from mock!" {
		t.Errorf("file content = %q, want %q", string(content), "Hello from mock!")
	}

	// Verify the streamed text is the final response.
	if handler.fullText() != "I've written the file for you." {
		t.Errorf("response text = %q", handler.fullText())
	}

	// History: user message + assistant (tool_use) + user (tool_result) + assistant (text).
	if loop.History().Len() != 4 {
		t.Errorf("history length = %d, want 4", loop.History().Len())
	}

	// Backend should have received 2 requests (one for each API call).
	if b.RequestCount() != 2 {
		t.Errorf("backend requests = %d, want 2", b.RequestCount())
	}
}

// --- E2E: FileRead tool ---

func TestE2E_FileReadTool(t *testing.T) {
	workDir := t.TempDir()
	filePath := filepath.Join(workDir, "readme.md")
	os.WriteFile(filePath, []byte("# Title\nSome content here."), 0644)

	readInput, _ := json.Marshal(map[string]interface{}{
		"file_path": filePath,
	})

	responder := mock.NewScriptedResponder([]*api.MessageResponse{
		mock.ToolUseResponse("toolu_read_1", "FileRead", readInput, 1),
		mock.TextResponse("The file contains a title and some content.", 2),
	})

	handler := &collectingHandler{}
	b := mock.NewBackend(responder)
	t.Cleanup(b.Close)

	registry := tools.NewRegistry(&tools.AlwaysAllowPermissionHandler{})
	registry.Register(tools.NewFileReadTool())

	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:   b.Client(),
		Tools:    registry.Definitions(),
		ToolExec: registry,
		Handler:  handler,
	})

	err := loop.SendMessage(context.Background(), "Read readme.md")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if handler.fullText() != "The file contains a title and some content." {
		t.Errorf("response = %q", handler.fullText())
	}

	// Verify the second request includes tool results.
	reqs := b.Requests()
	if len(reqs) != 2 {
		t.Fatalf("request count = %d", len(reqs))
	}

	// The second request should have more messages (original + assistant + tool result).
	if len(reqs[1].Body.Messages) != 3 {
		t.Errorf("second request messages = %d, want 3", len(reqs[1].Body.Messages))
	}
}

// --- E2E: Glob tool ---

func TestE2E_GlobTool(t *testing.T) {
	workDir := t.TempDir()
	os.WriteFile(filepath.Join(workDir, "a.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(workDir, "b.go"), []byte("package b"), 0644)
	os.WriteFile(filepath.Join(workDir, "c.txt"), []byte("text"), 0644)

	globInput, _ := json.Marshal(map[string]interface{}{
		"pattern": "*.go",
		"path":    workDir,
	})

	responder := mock.NewScriptedResponder([]*api.MessageResponse{
		mock.ToolUseResponse("toolu_glob_1", "Glob", globInput, 1),
		mock.TextResponse("Found 2 Go files.", 2),
	})

	handler := &collectingHandler{}
	b := mock.NewBackend(responder)
	t.Cleanup(b.Close)

	registry := tools.NewRegistry(&tools.AlwaysAllowPermissionHandler{})
	registry.Register(tools.NewGlobTool(workDir))

	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:   b.Client(),
		Tools:    registry.Definitions(),
		ToolExec: registry,
		Handler:  handler,
	})

	err := loop.SendMessage(context.Background(), "Find all Go files")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if handler.fullText() != "Found 2 Go files." {
		t.Errorf("response = %q", handler.fullText())
	}
}

// --- E2E: multiple tool calls in one turn ---

func TestE2E_MultipleToolCalls(t *testing.T) {
	workDir := t.TempDir()
	os.WriteFile(filepath.Join(workDir, "main.go"), []byte("package main\nfunc main() {}"), 0644)

	globInput, _ := json.Marshal(map[string]interface{}{
		"pattern": "*.go",
		"path":    workDir,
	})
	grepInput, _ := json.Marshal(map[string]interface{}{
		"pattern": "func main",
		"path":    workDir,
	})

	responder := mock.NewScriptedResponder([]*api.MessageResponse{
		// Turn 1: model requests Glob and Grep in parallel.
		mock.MultiToolUseResponse([]mock.ToolCall{
			{ID: "t1", Name: "Glob", Input: globInput},
			{ID: "t2", Name: "Grep", Input: grepInput},
		}, 1),
		// Turn 2: final response.
		mock.TextResponse("Found main.go with a main function.", 2),
	})

	handler := &collectingHandler{}
	b := mock.NewBackend(responder)
	t.Cleanup(b.Close)

	registry := tools.NewRegistry(&tools.AlwaysAllowPermissionHandler{})
	registry.Register(tools.NewGlobTool(workDir))
	registry.Register(tools.NewGrepTool(workDir))

	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:   b.Client(),
		Tools:    registry.Definitions(),
		ToolExec: registry,
		Handler:  handler,
	})

	err := loop.SendMessage(context.Background(), "Find Go files with main function")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Both tools should have executed and results sent back.
	if b.RequestCount() != 2 {
		t.Errorf("request count = %d, want 2", b.RequestCount())
	}

	if handler.fullText() != "Found main.go with a main function." {
		t.Errorf("response = %q", handler.fullText())
	}

	// History: user + assistant (2 tools) + user (2 tool results) + assistant (text) = 4.
	if loop.History().Len() != 4 {
		t.Errorf("history length = %d, want 4", loop.History().Len())
	}
}

// --- E2E: multi-turn conversation ---

func TestE2E_MultiTurnConversation(t *testing.T) {
	responder := mock.NewScriptedResponder([]*api.MessageResponse{
		mock.TextResponse("I'm a mock assistant. What do you need?", 1),
		mock.TextResponse("Sure, I can help with that.", 2),
	})

	handler := &collectingHandler{}
	_, loop := setupLoop(t, responder, handler)

	// Turn 1.
	err := loop.SendMessage(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if handler.fullText() != "I'm a mock assistant. What do you need?" {
		t.Errorf("turn 1 text = %q", handler.fullText())
	}

	// Reset handler for turn 2.
	handler.texts = nil

	// Turn 2.
	err = loop.SendMessage(context.Background(), "Can you help me?")
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if handler.fullText() != "Sure, I can help with that." {
		t.Errorf("turn 2 text = %q", handler.fullText())
	}

	// History: user + assistant + user + assistant = 4.
	if loop.History().Len() != 4 {
		t.Errorf("history length = %d, want 4", loop.History().Len())
	}
}

// --- E2E: tool use chain (tool → tool → final) ---

func TestE2E_ToolChain(t *testing.T) {
	workDir := t.TempDir()
	filePath := filepath.Join(workDir, "test.txt")

	// Step 1: Write a file.
	writeInput, _ := json.Marshal(map[string]interface{}{
		"file_path": filePath,
		"content":   "test content",
	})
	// Step 2: Read the file back.
	readInput, _ := json.Marshal(map[string]interface{}{
		"file_path": filePath,
	})

	responder := mock.NewScriptedResponder([]*api.MessageResponse{
		mock.ToolUseWithTextResponse(
			"First, I'll create the file.",
			"toolu_w1", "FileWrite", writeInput, 1,
		),
		mock.ToolUseWithTextResponse(
			"Now let me verify it was written correctly.",
			"toolu_r1", "FileRead", readInput, 2,
		),
		mock.TextResponse("The file was created and verified successfully.", 3),
	})

	handler := &collectingHandler{}
	b := mock.NewBackend(responder)
	t.Cleanup(b.Close)

	registry := tools.NewRegistry(&tools.AlwaysAllowPermissionHandler{})
	registry.Register(tools.NewFileWriteTool())
	registry.Register(tools.NewFileReadTool())

	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:   b.Client(),
		Tools:    registry.Definitions(),
		ToolExec: registry,
		Handler:  handler,
	})

	err := loop.SendMessage(context.Background(), "Create and verify test.txt")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// 3 API calls: write-tool → read-tool → final text.
	if b.RequestCount() != 3 {
		t.Errorf("request count = %d, want 3", b.RequestCount())
	}

	// File should exist with correct content.
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("file content = %q", string(content))
	}

	// The final streamed text should include all text blocks.
	got := handler.fullText()
	if !strings.Contains(got, "First, I'll create the file.") {
		t.Errorf("missing write text in %q", got)
	}
	if !strings.Contains(got, "verify it was written correctly") {
		t.Errorf("missing read text in %q", got)
	}
	if !strings.Contains(got, "created and verified successfully") {
		t.Errorf("missing final text in %q", got)
	}
}

// --- E2E: unknown tool gracefully handled ---

func TestE2E_UnknownTool(t *testing.T) {
	input := json.RawMessage(`{"query":"test"}`)

	responder := mock.NewScriptedResponder([]*api.MessageResponse{
		// Model requests a tool that isn't registered.
		mock.ToolUseResponse("toolu_unknown", "NonExistentTool", input, 1),
		// Model recovers after seeing the error.
		mock.TextResponse("I see that tool isn't available. Let me try differently.", 2),
	})

	handler := &collectingHandler{}
	_, loop := setupLoop(t, responder, handler)

	err := loop.SendMessage(context.Background(), "Do something")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Should still get a response — the loop handles missing tools gracefully.
	if !strings.Contains(handler.fullText(), "isn't available") {
		t.Errorf("response = %q", handler.fullText())
	}
}

// --- E2E: ResponderFunc for custom logic ---

func TestE2E_ResponderFunc(t *testing.T) {
	callCount := 0
	responder := mock.ResponderFunc(func(req *api.CreateMessageRequest) *api.MessageResponse {
		callCount++
		// Count the messages in the request.
		n := len(req.Messages)
		return mock.TextResponse(
			"You sent "+strings.Repeat(".", n)+" messages.",
			callCount,
		)
	})

	handler := &collectingHandler{}
	_, loop := setupLoop(t, responder, handler)

	err := loop.SendMessage(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Should have 1 message (user) in the request.
	if handler.fullText() != "You sent . messages." {
		t.Errorf("response = %q", handler.fullText())
	}
}

// --- E2E: request inspection for tools sent ---

func TestE2E_ToolDefinitionsSentInRequest(t *testing.T) {
	handler := &collectingHandler{}
	b, loop := setupLoop(t, &mock.StaticResponder{
		Response: mock.TextResponse("ok", 1),
	}, handler)

	loop.SendMessage(context.Background(), "hi")

	req := b.LastRequest()
	if req == nil {
		t.Fatal("no request captured")
	}

	// Verify that tool definitions were sent.
	if len(req.Body.Tools) == 0 {
		t.Error("no tools sent in request")
	}

	// Check that at least the tools we registered are present.
	toolNames := make(map[string]bool)
	for _, td := range req.Body.Tools {
		toolNames[td.Name] = true
	}
	for _, expected := range []string{"FileRead", "FileWrite", "Glob", "Grep"} {
		if !toolNames[expected] {
			t.Errorf("tool %q not found in request", expected)
		}
	}
}

// --- E2E: session persistence callback ---

func TestE2E_OnTurnComplete(t *testing.T) {
	var savedMessages []api.Message
	turnCount := 0

	handler := &collectingHandler{}
	_, loop := setupLoop(t, &mock.StaticResponder{
		Response: mock.TextResponse("ok", 1),
	}, handler)

	loop.SetOnTurnComplete(func(h *conversation.History) {
		turnCount++
		savedMessages = h.Messages()
	})

	loop.SendMessage(context.Background(), "hello")

	if turnCount != 1 {
		t.Errorf("turn count = %d, want 1", turnCount)
	}
	if len(savedMessages) != 2 {
		t.Errorf("saved messages = %d, want 2", len(savedMessages))
	}
}
