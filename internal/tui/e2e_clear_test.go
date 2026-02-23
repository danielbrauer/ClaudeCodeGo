package tui

import (
	"context"
	"testing"

	"github.com/anthropics/claude-code-go/internal/mock"
	"github.com/anthropics/claude-code-go/internal/session"
	"github.com/anthropics/claude-code-go/internal/tools"
)

func TestE2E_ClearCommand_ResetsHistory(t *testing.T) {
	m, _ := testModel(t, withResponder(&mock.StaticResponder{
		Response: mock.TextResponse("Hello!", 1),
	}))

	// Send a message to populate history.
	err := m.loop.SendMessage(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if m.loop.History().Len() != 2 {
		t.Fatalf("history should have 2 messages, got %d", m.loop.History().Len())
	}

	result, _ := submitCommand(m, "/clear")

	if result.loop.History().Len() != 0 {
		t.Errorf("history should be empty after /clear, got %d", result.loop.History().Len())
	}
}

func TestE2E_ClearCommand_ResetsTokens(t *testing.T) {
	m, _ := testModel(t)

	cacheRead := 500
	m.tokens.addInput(1000, &cacheRead, nil)
	m.tokens.addOutput(200)

	result, _ := submitCommand(m, "/clear")

	if result.tokens.TotalInputTokens != 0 {
		t.Errorf("input tokens should be 0, got %d", result.tokens.TotalInputTokens)
	}
	if result.tokens.TotalOutputTokens != 0 {
		t.Errorf("output tokens should be 0, got %d", result.tokens.TotalOutputTokens)
	}
	if result.tokens.TotalCacheRead != 0 {
		t.Errorf("cache read should be 0, got %d", result.tokens.TotalCacheRead)
	}
}

func TestE2E_ClearCommand_ClearsTodos(t *testing.T) {
	m, _ := testModel(t)

	m.todos = []tools.TodoItem{
		{Content: "task1", Status: "pending", ActiveForm: "Doing task1"},
	}

	result, _ := submitCommand(m, "/clear")

	if len(result.todos) != 0 {
		t.Errorf("todos should be empty after /clear, got %d", len(result.todos))
	}
}

func TestE2E_ClearCommand_CreatesNewSession(t *testing.T) {
	sessDir := t.TempDir()
	store := session.NewStoreWithDir(sessDir)

	oldSession := &session.Session{
		ID:    "old-session-id",
		Model: "claude-sonnet-4-20250514",
		CWD:   "/tmp/test",
	}
	m, _ := testModel(t,
		withSessionStore(store),
		withSession(oldSession),
	)

	result, _ := submitCommand(m, "/clear")

	if result.session == nil {
		t.Fatal("session should not be nil after /clear")
	}
	if result.session.ID == "old-session-id" {
		t.Error("session ID should be different after /clear")
	}
	if result.session.Model != "claude-sonnet-4-20250514" {
		t.Errorf("session model should be preserved, got %q", result.session.Model)
	}
}

func TestE2E_ResetCommand_IsAliasForClear(t *testing.T) {
	m, _ := testModel(t, withResponder(&mock.StaticResponder{
		Response: mock.TextResponse("Hello!", 1),
	}))

	err := m.loop.SendMessage(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	result, _ := submitCommand(m, "/reset")

	if result.loop.History().Len() != 0 {
		t.Errorf("history should be empty after /reset, got %d", result.loop.History().Len())
	}
}

func TestE2E_NewCommand_IsAliasForClear(t *testing.T) {
	m, _ := testModel(t, withResponder(&mock.StaticResponder{
		Response: mock.TextResponse("Hello!", 1),
	}))

	err := m.loop.SendMessage(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	result, _ := submitCommand(m, "/new")

	if result.loop.History().Len() != 0 {
		t.Errorf("history should be empty after /new, got %d", result.loop.History().Len())
	}
}

func TestE2E_ClearCommand_SavesNewSession(t *testing.T) {
	sessDir := t.TempDir()
	store := session.NewStoreWithDir(sessDir)

	oldSession := &session.Session{
		ID:    "old-id",
		Model: "claude-sonnet-4-20250514",
		CWD:   "/tmp/test",
	}
	m, _ := testModel(t,
		withSessionStore(store),
		withSession(oldSession),
	)

	result, _ := submitCommand(m, "/clear")

	// The new session should have been saved to the store.
	sessions, err := store.List()
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("expected at least one session saved after /clear")
	}

	// The saved session should match the new session ID.
	found := false
	for _, s := range sessions {
		if s.ID == result.session.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("new session %q not found in store", result.session.ID)
	}
}
