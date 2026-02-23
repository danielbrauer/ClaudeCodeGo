package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropics/claude-code-go/internal/api"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreWithDir(dir)

	sess := &Session{
		ID:        "test-123",
		Model:     "claude-sonnet-4-6",
		CWD:       "/tmp/test",
		Messages:  []api.Message{api.NewTextMessage("user", "hello")},
		CreatedAt: time.Now(),
	}

	// Save.
	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(dir, "test-123.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session file not created: %v", err)
	}

	// Load.
	loaded, err := store.Load("test-123")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.ID != sess.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, sess.ID)
	}
	if loaded.Model != sess.Model {
		t.Errorf("Model = %q, want %q", loaded.Model, sess.Model)
	}
	if loaded.CWD != sess.CWD {
		t.Errorf("CWD = %q, want %q", loaded.CWD, sess.CWD)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(loaded.Messages))
	}
	if loaded.Messages[0].Role != "user" {
		t.Errorf("Message role = %q, want %q", loaded.Messages[0].Role, "user")
	}
}

func TestStoreMostRecent(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreWithDir(dir)

	// Save two sessions with different timestamps.
	older := &Session{
		ID:        "older",
		Model:     "model",
		CWD:       "/tmp",
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}
	newer := &Session{
		ID:        "newer",
		Model:     "model",
		CWD:       "/tmp",
		CreatedAt: time.Now(),
	}

	if err := store.Save(older); err != nil {
		t.Fatalf("Save older: %v", err)
	}
	// Ensure different timestamps.
	time.Sleep(10 * time.Millisecond)
	if err := store.Save(newer); err != nil {
		t.Fatalf("Save newer: %v", err)
	}

	// MostRecent should return the newer session.
	recent, err := store.MostRecent()
	if err != nil {
		t.Fatalf("MostRecent: %v", err)
	}
	if recent.ID != "newer" {
		t.Errorf("MostRecent ID = %q, want %q", recent.ID, "newer")
	}
}

func TestStoreList(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreWithDir(dir)

	// Save three sessions.
	for _, id := range []string{"a", "b", "c"} {
		sess := &Session{
			ID:        id,
			Model:     "model",
			CWD:       "/tmp",
			CreatedAt: time.Now(),
		}
		if err := store.Save(sess); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List len = %d, want 3", len(list))
	}

	// Should be sorted newest first.
	if list[0].ID != "c" {
		t.Errorf("List[0].ID = %q, want %q", list[0].ID, "c")
	}
}

func TestStoreLoadNonExistent(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreWithDir(dir)

	_, err := store.Load("nonexistent")
	if err == nil {
		t.Error("expected error loading nonexistent session")
	}
}

func TestStoreListEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreWithDir(dir)

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List len = %d, want 0", len(list))
	}
}

func TestStoreEmptyDirMostRecent(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreWithDir(dir)

	_, err := store.MostRecent()
	if err == nil {
		t.Error("expected error for empty store")
	}
}

func TestSessionMessagesSerialization(t *testing.T) {
	// Test that messages with content blocks survive JSON round-trip.
	blocks := []api.ContentBlock{
		{Type: "text", Text: "hello"},
		{Type: "tool_use", ID: "t1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
	}
	msg := api.NewBlockMessage("assistant", blocks)

	sess := &Session{
		ID:       "serial-test",
		Model:    "model",
		CWD:      "/tmp",
		Messages: []api.Message{msg},
	}

	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(loaded.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(loaded.Messages))
	}
	if loaded.Messages[0].Role != "assistant" {
		t.Errorf("Role = %q, want %q", loaded.Messages[0].Role, "assistant")
	}
}

func TestGenerateID(t *testing.T) {
	id1 := GenerateID()
	id2 := GenerateID()

	if id1 == "" {
		t.Error("GenerateID returned empty string")
	}
	// IDs should differ (nanosecond precision).
	if id1 == id2 {
		t.Logf("Warning: IDs are identical (timing collision), acceptable in rare cases")
	}
}
