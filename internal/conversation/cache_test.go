package conversation

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
)

func TestIsCachingEnabled_Default(t *testing.T) {
	// With no env vars set, caching should be enabled for all models.
	for _, model := range []string{
		api.ModelClaude46Opus,
		api.ModelClaude46Sonnet,
		api.ModelClaude45Haiku,
		"unknown-model",
	} {
		if !IsCachingEnabled(model) {
			t.Errorf("IsCachingEnabled(%q) = false, want true (default)", model)
		}
	}
}

func TestIsCachingEnabled_DisableAll(t *testing.T) {
	t.Setenv("DISABLE_PROMPT_CACHING", "1")

	for _, model := range []string{
		api.ModelClaude46Opus,
		api.ModelClaude46Sonnet,
		api.ModelClaude45Haiku,
	} {
		if IsCachingEnabled(model) {
			t.Errorf("IsCachingEnabled(%q) = true, want false (globally disabled)", model)
		}
	}
}

func TestIsCachingEnabled_DisableAllTrue(t *testing.T) {
	t.Setenv("DISABLE_PROMPT_CACHING", "true")

	if IsCachingEnabled(api.ModelClaude46Opus) {
		t.Error("IsCachingEnabled should be false with DISABLE_PROMPT_CACHING=true")
	}
}

func TestIsCachingEnabled_DisablePerModel(t *testing.T) {
	tests := []struct {
		envVar       string
		disabledModel string
		enabledModels []string
	}{
		{
			envVar:       "DISABLE_PROMPT_CACHING_HAIKU",
			disabledModel: api.ModelClaude45Haiku,
			enabledModels: []string{api.ModelClaude46Opus, api.ModelClaude46Sonnet},
		},
		{
			envVar:       "DISABLE_PROMPT_CACHING_SONNET",
			disabledModel: api.ModelClaude46Sonnet,
			enabledModels: []string{api.ModelClaude46Opus, api.ModelClaude45Haiku},
		},
		{
			envVar:       "DISABLE_PROMPT_CACHING_OPUS",
			disabledModel: api.ModelClaude46Opus,
			enabledModels: []string{api.ModelClaude46Sonnet, api.ModelClaude45Haiku},
		},
	}

	for _, tt := range tests {
		t.Run(tt.envVar, func(t *testing.T) {
			t.Setenv(tt.envVar, "1")

			if IsCachingEnabled(tt.disabledModel) {
				t.Errorf("IsCachingEnabled(%q) = true, want false (%s=1)", tt.disabledModel, tt.envVar)
			}
			for _, model := range tt.enabledModels {
				if !IsCachingEnabled(model) {
					t.Errorf("IsCachingEnabled(%q) = false, want true (%s should only disable %q)",
						model, tt.envVar, tt.disabledModel)
				}
			}
		})
	}
}

func TestWithSystemPromptCaching_Empty(t *testing.T) {
	result := WithSystemPromptCaching(nil)
	if result != nil {
		t.Error("WithSystemPromptCaching(nil) should return nil")
	}
}

func TestWithSystemPromptCaching_AddsControl(t *testing.T) {
	blocks := []api.SystemBlock{
		{Type: "text", Text: "identity"},
		{Type: "text", Text: "project"},
	}

	result := WithSystemPromptCaching(blocks)

	// Original should be unmodified.
	for i, b := range blocks {
		if b.CacheControl != nil {
			t.Errorf("original blocks[%d].CacheControl should be nil", i)
		}
	}

	// Result should have cache_control only on the last block.
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].CacheControl != nil {
		t.Errorf("result[0].CacheControl should be nil (not last block)")
	}
	if result[1].CacheControl == nil || result[1].CacheControl.Type != "ephemeral" {
		t.Errorf("result[1].CacheControl = %v, want ephemeral", result[1].CacheControl)
	}
	for i, b := range result {
		if b.Text != blocks[i].Text {
			t.Errorf("result[%d].Text = %q, want %q", i, b.Text, blocks[i].Text)
		}
	}
}

func TestWithToolsCaching_Empty(t *testing.T) {
	result := WithToolsCaching(nil)
	if result != nil {
		t.Error("WithToolsCaching(nil) should return nil")
	}
}

func TestWithToolsCaching_LastToolOnly(t *testing.T) {
	tools := []api.ToolDefinition{
		{Name: "Bash", Description: "run commands"},
		{Name: "Read", Description: "read files"},
		{Name: "Grep", Description: "search files"},
	}

	result := WithToolsCaching(tools)

	// Original should be unmodified.
	for i, tool := range tools {
		if tool.CacheControl != nil {
			t.Errorf("original tools[%d].CacheControl should be nil", i)
		}
	}

	// Only the last tool should have cache_control.
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}
	for i := 0; i < len(result)-1; i++ {
		if result[i].CacheControl != nil {
			t.Errorf("result[%d].CacheControl should be nil (not last tool)", i)
		}
	}
	last := result[len(result)-1]
	if last.CacheControl == nil || last.CacheControl.Type != "ephemeral" {
		t.Errorf("last tool CacheControl = %v, want ephemeral", last.CacheControl)
	}
}

func TestWithToolsCaching_SingleTool(t *testing.T) {
	tools := []api.ToolDefinition{
		{Name: "Bash", Description: "run commands"},
	}

	result := WithToolsCaching(tools)

	if result[0].CacheControl == nil || result[0].CacheControl.Type != "ephemeral" {
		t.Errorf("single tool CacheControl = %v, want ephemeral", result[0].CacheControl)
	}
}

func TestWithMessageCaching_Empty(t *testing.T) {
	result := WithMessageCaching(nil)
	if result != nil {
		t.Error("WithMessageCaching(nil) should return nil")
	}
}

func TestWithMessageCaching_SingleMessage(t *testing.T) {
	msgs := []api.Message{
		api.NewTextMessage(api.RoleUser, "hello"),
	}

	result := WithMessageCaching(msgs)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}

	// The single message should have cache_control.
	var blocks []api.ContentBlock
	if err := json.Unmarshal(result[0].Content, &blocks); err != nil {
		t.Fatalf("unmarshal result content: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].CacheControl == nil || blocks[0].CacheControl.Type != "ephemeral" {
		t.Errorf("block CacheControl = %v, want ephemeral", blocks[0].CacheControl)
	}
	if blocks[0].Text != "hello" {
		t.Errorf("block Text = %q, want %q", blocks[0].Text, "hello")
	}
}

func TestWithMessageCaching_Last2Messages(t *testing.T) {
	msgs := []api.Message{
		api.NewTextMessage(api.RoleUser, "first"),
		api.NewTextMessage(api.RoleAssistant, "response1"),
		api.NewTextMessage(api.RoleUser, "second"),
		api.NewTextMessage(api.RoleAssistant, "response2"),
		api.NewTextMessage(api.RoleUser, "third"),
	}

	result := WithMessageCaching(msgs)

	if len(result) != 5 {
		t.Fatalf("len(result) = %d, want 5", len(result))
	}

	// First 3 messages should NOT have cache_control.
	for i := 0; i < 3; i++ {
		var blocks []api.ContentBlock
		// Original text messages are plain strings, not blocks.
		var text string
		if err := json.Unmarshal(result[i].Content, &text); err == nil {
			// Still a plain string = no cache_control applied (correct).
			continue
		}
		if err := json.Unmarshal(result[i].Content, &blocks); err == nil {
			for _, b := range blocks {
				if b.CacheControl != nil {
					t.Errorf("msgs[%d] should not have cache_control", i)
				}
			}
		}
	}

	// Last 2 messages (indices 3 and 4) should have cache_control.
	for i := 3; i < 5; i++ {
		var blocks []api.ContentBlock
		if err := json.Unmarshal(result[i].Content, &blocks); err != nil {
			t.Fatalf("msgs[%d]: unmarshal content: %v", i, err)
		}
		found := false
		for _, b := range blocks {
			if b.CacheControl != nil && b.CacheControl.Type == "ephemeral" {
				found = true
			}
		}
		if !found {
			t.Errorf("msgs[%d] should have cache_control on last block", i)
		}
	}
}

func TestWithMessageCaching_BlockMessages(t *testing.T) {
	// Create a message with multiple content blocks.
	blocks := []api.ContentBlock{
		{Type: api.ContentTypeText, Text: "first block"},
		{Type: api.ContentTypeText, Text: "second block"},
		{Type: api.ContentTypeText, Text: "third block"},
	}
	msgs := []api.Message{
		api.NewBlockMessage(api.RoleUser, blocks),
	}

	result := WithMessageCaching(msgs)

	var resultBlocks []api.ContentBlock
	if err := json.Unmarshal(result[0].Content, &resultBlocks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resultBlocks) != 3 {
		t.Fatalf("len(resultBlocks) = %d, want 3", len(resultBlocks))
	}

	// Only the last block should have cache_control.
	for i := 0; i < 2; i++ {
		if resultBlocks[i].CacheControl != nil {
			t.Errorf("resultBlocks[%d].CacheControl should be nil", i)
		}
	}
	if resultBlocks[2].CacheControl == nil || resultBlocks[2].CacheControl.Type != "ephemeral" {
		t.Errorf("last block CacheControl = %v, want ephemeral", resultBlocks[2].CacheControl)
	}
}

func TestWithMessageCaching_SkipsThinkingBlocks(t *testing.T) {
	// Create assistant message with thinking + text blocks.
	blocks := []api.ContentBlock{
		{Type: "thinking", Text: "let me think..."},
		{Type: api.ContentTypeText, Text: "here's my answer"},
		{Type: "redacted_thinking", Text: "[redacted]"},
	}
	msgs := []api.Message{
		api.NewBlockMessage(api.RoleAssistant, blocks),
	}

	result := WithMessageCaching(msgs)

	var resultBlocks []api.ContentBlock
	if err := json.Unmarshal(result[0].Content, &resultBlocks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// cache_control should be on the text block (index 1), not the thinking blocks.
	if resultBlocks[0].CacheControl != nil {
		t.Error("thinking block should not have cache_control")
	}
	if resultBlocks[1].CacheControl == nil || resultBlocks[1].CacheControl.Type != "ephemeral" {
		t.Errorf("text block CacheControl = %v, want ephemeral", resultBlocks[1].CacheControl)
	}
	if resultBlocks[2].CacheControl != nil {
		t.Error("redacted_thinking block should not have cache_control")
	}
}

func TestWithMessageCaching_AllThinkingBlocks(t *testing.T) {
	// Edge case: message with only thinking blocks.
	blocks := []api.ContentBlock{
		{Type: "thinking", Text: "hmm"},
		{Type: "redacted_thinking", Text: "[redacted]"},
	}
	msgs := []api.Message{
		api.NewBlockMessage(api.RoleAssistant, blocks),
	}

	result := WithMessageCaching(msgs)

	var resultBlocks []api.ContentBlock
	if err := json.Unmarshal(result[0].Content, &resultBlocks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// No blocks should have cache_control since they're all thinking.
	for i, b := range resultBlocks {
		if b.CacheControl != nil {
			t.Errorf("resultBlocks[%d].CacheControl should be nil (all thinking)", i)
		}
	}
}

func TestWithMessageCaching_DoesNotMutateOriginal(t *testing.T) {
	msgs := []api.Message{
		api.NewTextMessage(api.RoleUser, "hello"),
	}

	// Save original content.
	origContent := make([]byte, len(msgs[0].Content))
	copy(origContent, msgs[0].Content)

	_ = WithMessageCaching(msgs)

	// Original should be unchanged.
	if string(msgs[0].Content) != string(origContent) {
		t.Errorf("original message content was mutated: %s != %s",
			string(msgs[0].Content), string(origContent))
	}
}

func TestWithMessageCaching_ToolResultMessage(t *testing.T) {
	// Tool result messages contain tool_result blocks.
	blocks := []api.ContentBlock{
		{
			Type:      api.ContentTypeToolResult,
			ToolUseID: "tool_123",
			Content:   json.RawMessage(`"result output"`),
		},
	}
	msgs := []api.Message{
		api.NewBlockMessage(api.RoleUser, blocks),
	}

	result := WithMessageCaching(msgs)

	var resultBlocks []api.ContentBlock
	if err := json.Unmarshal(result[0].Content, &resultBlocks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resultBlocks[0].CacheControl == nil || resultBlocks[0].CacheControl.Type != "ephemeral" {
		t.Errorf("tool_result block CacheControl = %v, want ephemeral", resultBlocks[0].CacheControl)
	}
	if resultBlocks[0].ToolUseID != "tool_123" {
		t.Errorf("ToolUseID = %q, want %q", resultBlocks[0].ToolUseID, "tool_123")
	}
}

func TestWithSystemPromptCaching_DoesNotMutateOriginal(t *testing.T) {
	blocks := []api.SystemBlock{
		{Type: "text", Text: "identity"},
	}

	_ = WithSystemPromptCaching(blocks)

	if blocks[0].CacheControl != nil {
		t.Error("original blocks[0].CacheControl should be nil after WithSystemPromptCaching")
	}
}

func TestWithToolsCaching_DoesNotMutateOriginal(t *testing.T) {
	tools := []api.ToolDefinition{
		{Name: "Bash", Description: "run commands"},
	}

	_ = WithToolsCaching(tools)

	if tools[0].CacheControl != nil {
		t.Error("original tools[0].CacheControl should be nil after WithToolsCaching")
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"0", false},
		{"false", false},
		{"", false},
		{"yes", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("TEST_ENV_BOOL", tt.value)
			if got := envBool("TEST_ENV_BOOL"); got != tt.want {
				t.Errorf("envBool(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestEnvBool_Unset(t *testing.T) {
	// Ensure the var is not set.
	if got := envBool("DEFINITELY_NOT_SET_12345"); got {
		t.Error("envBool for unset var should be false")
	}
}
