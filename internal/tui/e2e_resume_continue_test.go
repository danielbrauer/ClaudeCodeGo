package tui

import (
	"testing"
	"time"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/session"
)

func TestE2E_ResumeCommand_NoSessionStore(t *testing.T) {
	m, _ := testModel(t) // no session store

	result, _ := submitCommand(m, "/resume")

	// Should remain in input mode (error printed).
	if result.mode != modeInput {
		t.Errorf("mode = %d, want modeInput (no session store)", result.mode)
	}
}

func TestE2E_ResumeCommand_NoSessions(t *testing.T) {
	store := session.NewStoreWithDir(t.TempDir())
	m, _ := testModel(t, withSessionStore(store))

	result, _ := submitCommand(m, "/resume")

	// Should remain in input mode (no sessions found).
	if result.mode != modeInput {
		t.Errorf("mode = %d, want modeInput (no sessions)", result.mode)
	}
}

func TestE2E_ResumeCommand_OpensPicker(t *testing.T) {
	store := session.NewStoreWithDir(t.TempDir())

	// Create some test sessions.
	sess1 := &session.Session{
		ID:        "sess-1",
		Model:     "claude-sonnet-4-20250514",
		CWD:       "/tmp",
		Messages:  []api.Message{makeTextMsg(api.RoleUser, "Hello")},
		UpdatedAt: time.Now(),
	}
	sess2 := &session.Session{
		ID:        "sess-2",
		Model:     "claude-sonnet-4-20250514",
		CWD:       "/tmp",
		Messages:  []api.Message{makeTextMsg(api.RoleUser, "World")},
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	store.Save(sess1)
	store.Save(sess2)

	m, _ := testModel(t, withSessionStore(store))

	result, _ := submitCommand(m, "/resume")

	if result.mode != modeResume {
		t.Errorf("mode = %d, want modeResume (%d)", result.mode, modeResume)
	}
	if len(result.resumeSessions) != 2 {
		t.Errorf("resumeSessions = %d, want 2", len(result.resumeSessions))
	}
	if result.resumeCursor != 0 {
		t.Errorf("resumeCursor = %d, want 0", result.resumeCursor)
	}
}

func TestE2E_ContinueCommand_NoSessionStore(t *testing.T) {
	m, _ := testModel(t) // no session store

	result, _ := submitCommand(m, "/continue")

	// Should remain in input mode (error printed).
	if result.mode != modeInput {
		t.Errorf("mode = %d, want modeInput (no session store)", result.mode)
	}
}

func TestE2E_ContinueCommand_NoSessions(t *testing.T) {
	store := session.NewStoreWithDir(t.TempDir())
	m, _ := testModel(t, withSessionStore(store))

	result, _ := submitCommand(m, "/continue")

	// Should remain in input mode (no sessions found).
	if result.mode != modeInput {
		t.Errorf("mode = %d, want modeInput (no sessions)", result.mode)
	}
}

func TestE2E_ContinueCommand_LoadsMostRecent(t *testing.T) {
	store := session.NewStoreWithDir(t.TempDir())

	sess1 := &session.Session{
		ID:        "sess-1",
		Model:     "claude-sonnet-4-20250514",
		CWD:       "/tmp",
		Messages:  []api.Message{makeTextMsg(api.RoleUser, "Hello")},
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	sess2 := &session.Session{
		ID:        "sess-2",
		Model:     "claude-sonnet-4-20250514",
		CWD:       "/tmp",
		Messages:  []api.Message{makeTextMsg(api.RoleUser, "World"), makeTextMsg(api.RoleAssistant, "Hi")},
		UpdatedAt: time.Now(),
	}
	store.Save(sess1)
	store.Save(sess2)

	currentSess := &session.Session{
		ID:    "current",
		Model: "claude-sonnet-4-20250514",
		CWD:   "/tmp",
	}
	m, _ := testModel(t,
		withSessionStore(store),
		withSession(currentSess),
	)

	result, _ := submitCommand(m, "/continue")

	// Should load the most recent session (sess-2).
	if result.session.ID != "sess-2" {
		t.Errorf("session ID = %q, want sess-2", result.session.ID)
	}

	// History should be restored.
	if result.loop.History().Len() != 2 {
		t.Errorf("history len = %d, want 2", result.loop.History().Len())
	}
}

func TestE2E_ContinueCommand_PreservesSessionFields(t *testing.T) {
	store := session.NewStoreWithDir(t.TempDir())

	now := time.Now()
	sess := &session.Session{
		ID:        "test-sess",
		Model:     "claude-opus-4-6-20250219",
		CWD:       "/home/user/project",
		Messages:  []api.Message{makeTextMsg(api.RoleUser, "test")},
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now,
	}
	store.Save(sess)

	currentSess := &session.Session{
		ID:    "current",
		Model: "claude-sonnet-4-20250514",
		CWD:   "/tmp",
	}
	m, _ := testModel(t,
		withSessionStore(store),
		withSession(currentSess),
	)

	result, _ := submitCommand(m, "/continue")

	if result.session.Model != "claude-opus-4-6-20250219" {
		t.Errorf("session Model = %q, want claude-opus-4-6-20250219", result.session.Model)
	}
	if result.session.CWD != "/home/user/project" {
		t.Errorf("session CWD = %q, want /home/user/project", result.session.CWD)
	}
}
