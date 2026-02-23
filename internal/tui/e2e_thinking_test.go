package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
	"github.com/anthropics/claude-code-go/internal/mock"
)

// ===========================================================================
// E2E: Thinking mode defaults and config panel toggle
// ===========================================================================

func TestE2E_ThinkingMode_DefaultEnabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAX_THINKING_TOKENS", "")
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "")

	settings := &config.Settings{}
	m, _ := testModel(t,
		withModelName(api.ModelClaude46Opus),
		withSettings(settings),
	)

	// Thinking should be enabled by default in the loop.
	if !m.loop.ThinkingEnabled() {
		t.Error("thinking should be enabled by default")
	}
}

func TestE2E_ThinkingMode_RespectsSettingFalse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAX_THINKING_TOKENS", "")
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "")

	disabled := false
	settings := &config.Settings{ThinkingEnabled: &disabled}
	m, _ := testModel(t,
		withModelName(api.ModelClaude46Opus),
		withSettings(settings),
		withThinkingEnabled(&disabled),
	)

	if m.loop.ThinkingEnabled() {
		t.Error("thinking should be disabled when setting is false")
	}
}

func TestE2E_ThinkingMode_ConfigPanelToggle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	settings := &config.Settings{}
	m, _ := testModel(t,
		withModelName(api.ModelClaude46Opus),
		withSettings(settings),
	)

	// Open config panel.
	result, _ := submitCommand(m, "/config")
	if result.configPanel == nil {
		t.Fatal("configPanel should be non-nil")
	}

	// Find the thinking setting and verify default is true.
	item := findItem(result.configPanel, "alwaysThinkingEnabled")
	if item == nil {
		t.Fatal("alwaysThinkingEnabled not found in config panel")
	}
	val := result.configPanel.getValue(*item)
	if val != "true" {
		t.Errorf("default thinking value = %q, want 'true'", val)
	}

	// Toggle it off.
	result.configPanel.toggleBool("alwaysThinkingEnabled", settings)
	if settings.ThinkingEnabled == nil || *settings.ThinkingEnabled != false {
		t.Errorf("ThinkingEnabled = %v, want false after toggle", settings.ThinkingEnabled)
	}

	// Close config panel — should sync to loop.
	result.settings = settings
	closed, _ := result.closeConfigPanel()
	result = closed.(model)
	if result.loop.ThinkingEnabled() {
		t.Error("loop ThinkingEnabled should be false after config panel close")
	}
}

func TestE2E_ThinkingMode_ConfigPanelShowsInSearch(t *testing.T) {
	settings := &config.Settings{}
	m, _ := testModel(t, withSettings(settings))

	result, _ := submitCommand(m, "/config")
	cp := result.configPanel
	if cp == nil {
		t.Fatal("configPanel is nil")
	}

	// Searching for "thinking" should find the setting.
	cp.searchQuery = "thinking"
	cp.applyFilter()

	if len(cp.filtered) != 1 {
		t.Errorf("filtered for 'thinking': len = %d, want 1", len(cp.filtered))
	}
	if len(cp.filtered) > 0 {
		item := cp.items[cp.filtered[0]]
		if item.id != "alwaysThinkingEnabled" {
			t.Errorf("expected alwaysThinkingEnabled, got %q", item.id)
		}
	}
}

// ===========================================================================
// E2E: Thinking config sent to the API
// ===========================================================================

func TestE2E_ThinkingMode_AdaptiveSentForOpus46(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAX_THINKING_TOKENS", "")
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "")

	m, backend := testModel(t,
		withModelName(api.ModelClaude46Opus),
		withResponder(&mock.StaticResponder{
			Response: mock.TextResponse("hello", 1),
		}),
	)

	// Send a message and wait for the loop to complete.
	result, cmd := submitCommand(m, "test thinking")
	runLoopCmd(t, result, cmd)

	// Verify the API request includes adaptive thinking.
	req := backend.LastRequest()
	if req == nil {
		t.Fatal("no request captured")
	}
	if req.Body.Thinking == nil {
		t.Fatal("thinking should be set in the request")
	}
	if req.Body.Thinking.Type != "adaptive" {
		t.Errorf("thinking.type = %q, want 'adaptive'", req.Body.Thinking.Type)
	}

	// Verify the adaptive thinking beta header is present.
	beta := req.Headers.Get("Anthropic-Beta")
	if !strings.Contains(beta, api.AdaptiveThinkingBeta) {
		t.Errorf("beta header should contain %q, got %q", api.AdaptiveThinkingBeta, beta)
	}
}

func TestE2E_ThinkingMode_BudgetSentForOlderModels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAX_THINKING_TOKENS", "")
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "")

	m, backend := testModel(t,
		withModelName("claude-sonnet-4-20250514"),
		withResponder(&mock.StaticResponder{
			Response: mock.TextResponse("hello", 1),
		}),
	)

	result, cmd := submitCommand(m, "test thinking budget")
	runLoopCmd(t, result, cmd)

	req := backend.LastRequest()
	if req == nil {
		t.Fatal("no request captured")
	}
	if req.Body.Thinking == nil {
		t.Fatal("thinking should be set for Sonnet 4")
	}
	if req.Body.Thinking.Type != "enabled" {
		t.Errorf("thinking.type = %q, want 'enabled'", req.Body.Thinking.Type)
	}
	if req.Body.Thinking.BudgetTokens != api.DefaultMaxTokens-1 {
		t.Errorf("budget_tokens = %d, want %d", req.Body.Thinking.BudgetTokens, api.DefaultMaxTokens-1)
	}

	// Adaptive beta should NOT be present for budget-based thinking.
	beta := req.Headers.Get("Anthropic-Beta")
	if strings.Contains(beta, api.AdaptiveThinkingBeta) {
		t.Errorf("adaptive beta should not be present for budget-based thinking, got %q", beta)
	}
}

func TestE2E_ThinkingMode_OmittedWhenDisabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAX_THINKING_TOKENS", "")
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "")

	disabled := false
	m, backend := testModel(t,
		withModelName(api.ModelClaude46Opus),
		withThinkingEnabled(&disabled),
		withResponder(&mock.StaticResponder{
			Response: mock.TextResponse("hello", 1),
		}),
	)

	result, cmd := submitCommand(m, "test no thinking")
	runLoopCmd(t, result, cmd)

	req := backend.LastRequest()
	if req == nil {
		t.Fatal("no request captured")
	}
	if req.Body.Thinking != nil {
		t.Errorf("thinking should be nil when disabled, got %+v", req.Body.Thinking)
	}
}

func TestE2E_ThinkingMode_DisabledByEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAX_THINKING_TOKENS", "")
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "1")

	m, backend := testModel(t,
		withModelName(api.ModelClaude46Opus),
		withResponder(&mock.StaticResponder{
			Response: mock.TextResponse("hello", 1),
		}),
	)

	result, cmd := submitCommand(m, "test env disable")
	runLoopCmd(t, result, cmd)

	req := backend.LastRequest()
	if req == nil {
		t.Fatal("no request captured")
	}
	if req.Body.Thinking != nil {
		t.Errorf("thinking should be nil with CLAUDE_CODE_DISABLE_THINKING=1, got %+v", req.Body.Thinking)
	}
}

func TestE2E_ThinkingMode_MaxTokensEnvOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAX_THINKING_TOKENS", "5000")
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "")

	m, backend := testModel(t,
		withModelName(api.ModelClaude46Opus),
		withResponder(&mock.StaticResponder{
			Response: mock.TextResponse("hello", 1),
		}),
	)

	result, cmd := submitCommand(m, "test env override")
	runLoopCmd(t, result, cmd)

	req := backend.LastRequest()
	if req == nil {
		t.Fatal("no request captured")
	}
	if req.Body.Thinking == nil {
		t.Fatal("thinking should be set with MAX_THINKING_TOKENS")
	}
	if req.Body.Thinking.Type != "enabled" {
		t.Errorf("thinking.type = %q, want 'enabled' (env override bypasses adaptive)", req.Body.Thinking.Type)
	}
	if req.Body.Thinking.BudgetTokens != 5000 {
		t.Errorf("budget_tokens = %d, want 5000", req.Body.Thinking.BudgetTokens)
	}
}

func TestE2E_ThinkingMode_CoexistsWithFastMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAX_THINKING_TOKENS", "")
	t.Setenv("CLAUDE_CODE_DISABLE_THINKING", "")

	m, backend := testModel(t,
		withModelName(api.ModelClaude46Opus),
		withFastMode(true),
		withResponder(&mock.StaticResponder{
			Response: mock.TextResponse("hello", 1),
		}),
	)

	result, cmd := submitCommand(m, "test both modes")
	runLoopCmd(t, result, cmd)

	req := backend.LastRequest()
	if req == nil {
		t.Fatal("no request captured")
	}

	// Both should be present.
	if req.Body.Speed != "fast" {
		t.Errorf("speed = %q, want 'fast'", req.Body.Speed)
	}
	if req.Body.Thinking == nil || req.Body.Thinking.Type != "adaptive" {
		t.Errorf("thinking should be adaptive, got %+v", req.Body.Thinking)
	}

	// Both beta headers should be present.
	beta := req.Headers.Get("Anthropic-Beta")
	if !strings.Contains(beta, api.FastModeBeta) {
		t.Errorf("beta should contain fast mode beta, got %q", beta)
	}
	if !strings.Contains(beta, api.AdaptiveThinkingBeta) {
		t.Errorf("beta should contain adaptive thinking beta, got %q", beta)
	}
}

// ===========================================================================
// Helpers
// ===========================================================================

// runLoopCmd executes the command returned by submitCommand and drives the
// model through the streaming loop until LoopDoneMsg is received.
func runLoopCmd(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()

	if cmd == nil {
		return m
	}

	// Execute all batched commands and collect messages.
	msgs := executeBatchCmd(cmd)

	// Process messages until we get LoopDoneMsg.
	for _, msg := range msgs {
		if _, ok := msg.(LoopDoneMsg); ok {
			return m
		}
		result, newCmd := m.Update(msg)
		m = result.(model)
		if newCmd != nil {
			moreMsgs := executeBatchCmd(newCmd)
			msgs = append(msgs, moreMsgs...)
		}
	}

	return m
}

// executeBatchCmd runs a tea.Cmd and collects all resulting messages.
// It handles batch commands by recursively executing sub-commands.
func executeBatchCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	if msg == nil {
		return nil
	}

	// Check if it's a batch message (sequence of commands).
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, subCmd := range batchMsg {
			msgs = append(msgs, executeBatchCmd(subCmd)...)
		}
		return msgs
	}

	return []tea.Msg{msg}
}
