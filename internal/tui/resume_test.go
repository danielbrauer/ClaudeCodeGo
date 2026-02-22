package tui

import (
	"testing"
	"time"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/session"
)

func TestRelativeTime(t *testing.T) {
	tests := []struct {
		name string
		when time.Time
		want string
	}{
		{"just now", time.Now().Add(-10 * time.Second), "just now"},
		{"minutes", time.Now().Add(-5 * time.Minute), "5m ago"},
		{"hours", time.Now().Add(-3 * time.Hour), "3h ago"},
		{"days", time.Now().Add(-2 * 24 * time.Hour), "2d ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relativeTime(tt.when)
			if got != tt.want {
				t.Errorf("relativeTime(%v) = %q, want %q", tt.when, got, tt.want)
			}
		})
	}
}

func TestRelativeTimeOld(t *testing.T) {
	// Dates older than 30 days should show "Mon D" format.
	old := time.Now().Add(-60 * 24 * time.Hour)
	got := relativeTime(old)
	if got == "" {
		t.Error("relativeTime returned empty for old date")
	}
	// Should not contain "ago".
	if got[len(got)-3:] == "ago" {
		t.Errorf("relativeTime for old date should not end with 'ago': %q", got)
	}
}

func TestFirstUserMessage(t *testing.T) {
	tests := []struct {
		name string
		sess *session.Session
		want string
	}{
		{
			name: "simple text message",
			sess: &session.Session{
				Messages: []api.Message{
					api.NewTextMessage(api.RoleUser, "hello world"),
				},
			},
			want: "hello world",
		},
		{
			name: "assistant first then user",
			sess: &session.Session{
				Messages: []api.Message{
					api.NewTextMessage(api.RoleAssistant, "I am Claude"),
					api.NewTextMessage(api.RoleUser, "hi there"),
				},
			},
			want: "hi there",
		},
		{
			name: "empty session",
			sess: &session.Session{
				Messages: []api.Message{},
			},
			want: "",
		},
		{
			name: "block message with text",
			sess: &session.Session{
				Messages: []api.Message{
					api.NewBlockMessage(api.RoleUser, []api.ContentBlock{
						{Type: api.ContentTypeText, Text: "block text"},
					}),
				},
			},
			want: "block text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstUserMessage(tt.sess)
			if got != tt.want {
				t.Errorf("firstUserMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionSummary(t *testing.T) {
	sess := &session.Session{
		ID:        "test-123",
		UpdatedAt: time.Now().Add(-5 * time.Minute),
		Messages: []api.Message{
			api.NewTextMessage(api.RoleUser, "hello"),
			api.NewTextMessage(api.RoleAssistant, "hi"),
		},
	}

	got := sessionSummary(sess)
	if got != "5m ago, 2 messages" {
		t.Errorf("sessionSummary() = %q, want %q", got, "5m ago, 2 messages")
	}
}

func TestSessionSummarySingular(t *testing.T) {
	sess := &session.Session{
		ID:        "test-456",
		UpdatedAt: time.Now().Add(-1 * time.Minute),
		Messages: []api.Message{
			api.NewTextMessage(api.RoleUser, "hello"),
		},
	}

	got := sessionSummary(sess)
	if got != "1m ago, 1 message" {
		t.Errorf("sessionSummary() = %q, want %q", got, "1m ago, 1 message")
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		n    int
		s, p string
		want string
	}{
		{0, "message", "messages", "0 messages"},
		{1, "message", "messages", "1 message"},
		{5, "message", "messages", "5 messages"},
		{42, "", "", "42"},
	}

	for _, tt := range tests {
		got := pluralize(tt.n, tt.s, tt.p)
		if got != tt.want {
			t.Errorf("pluralize(%d, %q, %q) = %q, want %q", tt.n, tt.s, tt.p, got, tt.want)
		}
	}
}
