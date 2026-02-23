package tui

import (
	"context"
	"encoding/json"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/mock"
	"github.com/anthropics/claude-code-go/internal/session"
	"github.com/anthropics/claude-code-go/internal/skills"
)

// testModel creates a model wired to a mock backend for e2e testing.
// The model is in modeInput, ready to accept slash commands via handleSubmit.
func testModel(t *testing.T, opts ...testModelOption) (model, *mock.Backend) {
	t.Helper()

	cfg := testModelConfig{
		modelName: "claude-sonnet-4-20250514",
		version:   "1.0.0-test",
	}
	for _, o := range opts {
		o(&cfg)
	}

	var b *mock.Backend
	var client *api.Client

	if cfg.responder != nil {
		b = mock.NewBackend(cfg.responder)
		t.Cleanup(b.Close)
		client = b.Client(api.WithModel(cfg.modelName))
	} else {
		// Default: static text responder.
		b = mock.NewBackend(&mock.StaticResponder{
			Response: mock.TextResponse("ok", 1),
		})
		t.Cleanup(b.Close)
		client = b.Client(api.WithModel(cfg.modelName))
	}

	handler := &collectingStreamHandler{}
	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:          client,
		Handler:         handler,
		Compactor:       cfg.compactor,
		ThinkingEnabled: cfg.thinkingEnabled,
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	m := newModel(
		loop,
		ctx,
		cancel,
		cfg.modelName,
		cfg.version,
		"", // no initial prompt
		80, // width
		cfg.mcpStatus,
		cfg.skills,
		cfg.sessStore,
		cfg.session,
		cfg.settings,
		cfg.onModelSwitch,
		cfg.logoutFunc,
		cfg.fastMode,
	)
	m.apiClient = client

	// Sync fast mode to the loop, matching main.go behavior.
	loop.SetFastMode(cfg.fastMode)

	return m, b
}

// testModelConfig holds configuration options for testModel.
type testModelConfig struct {
	modelName       string
	version         string
	responder       mock.Responder
	mcpStatus       MCPStatus
	skills          []skills.Skill
	sessStore       *session.Store
	session         *session.Session
	settings        *config.Settings
	onModelSwitch   func(string)
	logoutFunc      func() error
	fastMode        bool
	compactor       *conversation.Compactor
	thinkingEnabled *bool // nil = default (true)
}

// testModelOption is a functional option for testModel.
type testModelOption func(*testModelConfig)

func withModelName(name string) testModelOption {
	return func(c *testModelConfig) { c.modelName = name }
}

func withVersion(v string) testModelOption {
	return func(c *testModelConfig) { c.version = v }
}

func withResponder(r mock.Responder) testModelOption {
	return func(c *testModelConfig) { c.responder = r }
}

func withMCPStatus(s MCPStatus) testModelOption {
	return func(c *testModelConfig) { c.mcpStatus = s }
}

func withSkills(s []skills.Skill) testModelOption {
	return func(c *testModelConfig) { c.skills = s }
}

func withSessionStore(s *session.Store) testModelOption {
	return func(c *testModelConfig) { c.sessStore = s }
}

func withSession(s *session.Session) testModelOption {
	return func(c *testModelConfig) { c.session = s }
}

func withSettings(s *config.Settings) testModelOption {
	return func(c *testModelConfig) { c.settings = s }
}

func withOnModelSwitch(fn func(string)) testModelOption {
	return func(c *testModelConfig) { c.onModelSwitch = fn }
}

func withLogoutFunc(fn func() error) testModelOption {
	return func(c *testModelConfig) { c.logoutFunc = fn }
}

func withFastMode(on bool) testModelOption {
	return func(c *testModelConfig) { c.fastMode = on }
}

func withCompactor(c *conversation.Compactor) testModelOption {
	return func(cfg *testModelConfig) { cfg.compactor = c }
}

func withThinkingEnabled(v *bool) testModelOption {
	return func(cfg *testModelConfig) { cfg.thinkingEnabled = v }
}

// collectingStreamHandler collects all streamed text for assertions.
type collectingStreamHandler struct {
	texts []string
}

func (h *collectingStreamHandler) OnMessageStart(_ api.MessageResponse)                  {}
func (h *collectingStreamHandler) OnContentBlockStart(_ int, _ api.ContentBlock)         {}
func (h *collectingStreamHandler) OnTextDelta(_ int, text string)                        { h.texts = append(h.texts, text) }
func (h *collectingStreamHandler) OnInputJSONDelta(_ int, _ string)                      {}
func (h *collectingStreamHandler) OnContentBlockStop(_ int)                              {}
func (h *collectingStreamHandler) OnMessageDelta(_ api.MessageDeltaBody, _ *api.Usage)   {}
func (h *collectingStreamHandler) OnMessageStop()                                        {}
func (h *collectingStreamHandler) OnError(_ error)                                       {}

// extractPrintlnTexts collects all tea.Println output text from the commands
// returned by handleSubmit or Update. This is used to verify what text a slash
// command writes to the scrollback.
//
// The tea.Println function returns a tea.Cmd that, when executed, produces a
// tea.printLineMessage(s). We can't inspect cmd functions directly, but we can
// execute them and inspect the resulting messages. However, Bubble Tea's
// tea.Println is implemented as a Sequence of Print+Println markers.
//
// Instead, we use a simpler approach: call handleSubmit, then inspect the model
// state changes and any direct output messages. For commands with Execute != nil,
// we call Execute directly and check the returned string.
//
// For full e2e tests, we check the model state after running the command.

// submitCommand simulates submitting a slash command and returns the resulting
// model and any batch commands. This exercises the full handleSubmit path.
func submitCommand(m model, text string) (model, tea.Cmd) {
	result, cmd := m.handleSubmit(text)
	return result.(model), cmd
}

// mockMCPStatus implements the MCPStatus interface for testing.
type mockMCPStatus struct {
	servers  []string
	statuses map[string]string
}

func (m *mockMCPStatus) Servers() []string { return m.servers }
func (m *mockMCPStatus) ServerStatus(name string) string {
	if s, ok := m.statuses[name]; ok {
		return s
	}
	return name + ": unknown"
}

// makeTestSession creates a test session with messages for testing.
func makeTestSession(id string, msgs ...api.Message) *session.Session {
	return &session.Session{
		ID:       id,
		Model:    "claude-sonnet-4-20250514",
		CWD:      "/tmp/test",
		Messages: msgs,
	}
}

// makeTextMsg creates a simple text message for testing.
func makeTextMsg(role, text string) api.Message {
	content, _ := json.Marshal(text)
	return api.Message{Role: role, Content: content}
}
