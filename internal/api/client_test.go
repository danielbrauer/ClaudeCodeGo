package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ===========================================================================
// Test helpers
// ===========================================================================

// staticTokenSource returns the same token every time.
type staticTokenSource struct {
	token string
}

func (s *staticTokenSource) GetAccessToken(_ context.Context) (string, error) {
	return s.token, nil
}

// refreshableTokenSource tracks invalidation calls and returns different
// tokens before and after invalidation.
type refreshableTokenSource struct {
	initialToken   string
	refreshedToken string
	invalidated    atomic.Bool
}

func (r *refreshableTokenSource) GetAccessToken(_ context.Context) (string, error) {
	if r.invalidated.Load() {
		return r.refreshedToken, nil
	}
	return r.initialToken, nil
}

func (r *refreshableTokenSource) InvalidateToken() {
	r.invalidated.Store(true)
}

// ===========================================================================
// Issue 14: User-Agent header on API requests
// ===========================================================================

func TestClient_UserAgentDefault(t *testing.T) {
	var capturedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.WriteHeader(200)
		// Return a minimal SSE stream so CreateMessageStream doesn't error.
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewClient(
		&staticTokenSource{token: "tok"},
		WithBaseURL(server.URL),
	)

	client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})

	if capturedUA != "claude-code/dev" {
		t.Errorf("default User-Agent: got %q, want %q", capturedUA, "claude-code/dev")
	}
}

func TestClient_UserAgentWithVersion(t *testing.T) {
	var capturedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.WriteHeader(200)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewClient(
		&staticTokenSource{token: "tok"},
		WithBaseURL(server.URL),
		WithVersion("1.2.3"),
	)

	client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})

	if capturedUA != "claude-code/1.2.3" {
		t.Errorf("versioned User-Agent: got %q, want %q", capturedUA, "claude-code/1.2.3")
	}
}

func TestWithVersion(t *testing.T) {
	client := NewClient(&staticTokenSource{token: "t"}, WithVersion("2.0.0"))
	if client.userAgent != "claude-code/2.0.0" {
		t.Errorf("userAgent: got %q", client.userAgent)
	}
}

// ===========================================================================
// Issue 16: Accept header should be application/json, not text/event-stream
// ===========================================================================

func TestClient_AcceptHeaderJSON(t *testing.T) {
	var capturedAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAccept = r.Header.Get("Accept")
		w.WriteHeader(200)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewClient(
		&staticTokenSource{token: "tok"},
		WithBaseURL(server.URL),
	)

	client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})

	if capturedAccept != "application/json" {
		t.Errorf("Accept header: got %q, want %q", capturedAccept, "application/json")
	}
}

func TestClient_AcceptHeaderNotEventStream(t *testing.T) {
	var capturedAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAccept = r.Header.Get("Accept")
		w.WriteHeader(200)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewClient(
		&staticTokenSource{token: "tok"},
		WithBaseURL(server.URL),
	)

	client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})

	if capturedAccept == "text/event-stream" {
		t.Error("Accept header should not be text/event-stream")
	}
}

// ===========================================================================
// Issue 15: 401 auto-retry on API calls
// ===========================================================================

func TestClient_401RetriesWithRefreshedToken(t *testing.T) {
	var requestCount atomic.Int32
	var lastAuthHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		lastAuthHeader = r.Header.Get("Authorization")

		if count == 1 {
			// First request: return 401.
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"unauthorized"}`)
			return
		}
		// Second request (retry): return 200.
		w.WriteHeader(200)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	ts := &refreshableTokenSource{
		initialToken:   "old-token",
		refreshedToken: "new-token",
	}

	client := NewClient(ts, WithBaseURL(server.URL))

	_, err := client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify two requests were made.
	if count := requestCount.Load(); count != 2 {
		t.Errorf("expected 2 requests (original + retry), got %d", count)
	}

	// Verify token was invalidated and refreshed.
	if !ts.invalidated.Load() {
		t.Error("token should have been invalidated after 401")
	}

	// Verify retry used the new token.
	if lastAuthHeader != "Bearer new-token" {
		t.Errorf("retry should use refreshed token, got %q", lastAuthHeader)
	}
}

func TestClient_401NoRetryWithoutRefreshable(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":"unauthorized"}`)
	}))
	defer server.Close()

	// Use a plain (non-refreshable) token source.
	client := NewClient(
		&staticTokenSource{token: "tok"},
		WithBaseURL(server.URL),
	)

	_, err := client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})

	// Should get an error (401).
	if err == nil {
		t.Fatal("expected error for 401 without refreshable token source")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention 401: %v", err)
	}

	// Only one request should have been made (no retry).
	if count := requestCount.Load(); count != 1 {
		t.Errorf("expected 1 request (no retry without RefreshableTokenSource), got %d", count)
	}
}

func TestClient_401RetryOnlyOnce(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		// Always return 401.
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":"unauthorized"}`)
	}))
	defer server.Close()

	ts := &refreshableTokenSource{
		initialToken:   "tok1",
		refreshedToken: "tok2",
	}

	client := NewClient(ts, WithBaseURL(server.URL))

	_, err := client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})

	// Should fail after retry.
	if err == nil {
		t.Fatal("expected error after retry")
	}

	// Exactly 2 requests: original + one retry.
	if count := requestCount.Load(); count != 2 {
		t.Errorf("expected 2 requests max, got %d", count)
	}
}

func TestClient_NonAuthErrorNotRetried(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":"server error"}`)
	}))
	defer server.Close()

	ts := &refreshableTokenSource{
		initialToken:   "tok",
		refreshedToken: "new-tok",
	}

	client := NewClient(ts, WithBaseURL(server.URL))

	_, err := client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})

	if err == nil {
		t.Fatal("expected error for 500")
	}

	// Only one request — 500 is not retried.
	if count := requestCount.Load(); count != 1 {
		t.Errorf("expected 1 request (500 not retried), got %d", count)
	}

	// Token should NOT be invalidated.
	if ts.invalidated.Load() {
		t.Error("token should not be invalidated for non-401 errors")
	}
}

// ===========================================================================
// Verify all expected headers are sent
// ===========================================================================

func TestClient_AllHeaders(t *testing.T) {
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.WriteHeader(200)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewClient(
		&staticTokenSource{token: "my-token"},
		WithBaseURL(server.URL),
		WithVersion("3.0.0"),
	)

	client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})

	expected := map[string]string{
		"Authorization":    "Bearer my-token",
		"Content-Type":     "application/json",
		"Anthropic-Version": "2023-06-01",
		"Anthropic-Beta":   "claude-code-20250219,oauth-2025-04-20",
		"X-App":            "cli",
		"User-Agent":       "claude-code/3.0.0",
		"Accept":           "application/json",
	}

	for key, want := range expected {
		got := headers.Get(key)
		if got != want {
			t.Errorf("header %q: got %q, want %q", key, got, want)
		}
	}
}

// ===========================================================================
// IsOpus46Model
// ===========================================================================

func TestIsOpus46Model(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-opus-4-6-20250514", true},
		{"claude-opus-4-6-20260101", true},
		{"CLAUDE-OPUS-4-6-20250514", true},  // case insensitive
		{"claude-opus-4-20250514", false},    // Opus 4, not 4.6
		{"claude-sonnet-4-20250514", false},
		{"claude-3-5-haiku-20241022", false},
		{"opus", false},                      // alias, not full ID
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := IsOpus46Model(tt.model)
			if got != tt.want {
				t.Errorf("IsOpus46Model(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

// ===========================================================================
// SetModel
// ===========================================================================

func TestClient_SetModel(t *testing.T) {
	client := NewClient(&staticTokenSource{token: "t"})
	if client.Model() != ModelClaude46Opus {
		t.Fatalf("default model = %q", client.Model())
	}
	client.SetModel(ModelClaude46Sonnet)
	if client.Model() != ModelClaude46Sonnet {
		t.Errorf("after SetModel: got %q, want %q", client.Model(), ModelClaude46Sonnet)
	}
}

// ===========================================================================
// Speed field serialization
// ===========================================================================

func TestSpeedFieldSerialization(t *testing.T) {
	// speed:"fast" should appear in JSON.
	req := &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
		Speed:    "fast",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"speed":"fast"`) {
		t.Errorf("JSON should contain speed:fast, got: %s", data)
	}

	// Empty speed should be omitted.
	req2 := &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}
	data2, _ := json.Marshal(req2)
	if strings.Contains(string(data2), "speed") {
		t.Errorf("JSON should omit empty speed, got: %s", data2)
	}
}

// ===========================================================================
// Fast mode beta header
// ===========================================================================

func TestClient_FastModeBetaHeader(t *testing.T) {
	var capturedBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBeta = r.Header.Get("Anthropic-Beta")
		w.WriteHeader(200)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewClient(
		&staticTokenSource{token: "tok"},
		WithBaseURL(server.URL),
	)

	// Request with speed:"fast" should include the fast mode beta.
	client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
		Speed:    "fast",
	}, &testHandler{})

	if !strings.Contains(capturedBeta, FastModeBeta) {
		t.Errorf("beta header should contain %q, got %q", FastModeBeta, capturedBeta)
	}
	// Should still contain the base betas.
	if !strings.Contains(capturedBeta, "claude-code-20250219") {
		t.Errorf("beta header should contain claude-code-20250219, got %q", capturedBeta)
	}
}

func TestClient_NoFastModeBetaWithoutSpeed(t *testing.T) {
	var capturedBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBeta = r.Header.Get("Anthropic-Beta")
		w.WriteHeader(200)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	client := NewClient(
		&staticTokenSource{token: "tok"},
		WithBaseURL(server.URL),
	)

	// Request without speed should NOT include the fast mode beta.
	client.CreateMessageStream(context.Background(), &CreateMessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	}, &testHandler{})

	if strings.Contains(capturedBeta, FastModeBeta) {
		t.Errorf("beta header should NOT contain %q without speed:fast, got %q", FastModeBeta, capturedBeta)
	}
}

// ===========================================================================
// RefreshableTokenSource interface satisfaction
// ===========================================================================

func TestRefreshableTokenSource_Interface(t *testing.T) {
	var ts RefreshableTokenSource = &refreshableTokenSource{
		initialToken:   "a",
		refreshedToken: "b",
	}
	tok, err := ts.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if tok != "a" {
		t.Errorf("initial token: got %q", tok)
	}
	ts.InvalidateToken()
	tok, _ = ts.GetAccessToken(context.Background())
	if tok != "b" {
		t.Errorf("refreshed token: got %q", tok)
	}
}
