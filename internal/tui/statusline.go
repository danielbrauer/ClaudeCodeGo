package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/config"
)

// statusLineData is the JSON structure piped to the status line command's stdin.
// It matches the schema documented at https://code.claude.com/docs/en/statusline.
type statusLineData struct {
	Cwd           string              `json:"cwd"`
	SessionID     string              `json:"session_id"`
	Model         statusLineModel     `json:"model"`
	Workspace     statusLineWorkspace `json:"workspace"`
	Version       string              `json:"version"`
	OutputStyle   statusLineStyle     `json:"output_style"`
	Cost          statusLineCost      `json:"cost"`
	ContextWindow statusLineContext   `json:"context_window"`
}

type statusLineModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type statusLineWorkspace struct {
	CurrentDir string `json:"current_dir"`
	ProjectDir string `json:"project_dir"`
}

type statusLineStyle struct {
	Name string `json:"name"`
}

type statusLineCost struct {
	TotalCostUSD       float64 `json:"total_cost_usd"`
	TotalDurationMs    int64   `json:"total_duration_ms"`
	TotalAPIDurationMs int64   `json:"total_api_duration_ms"`
}

type statusLineContext struct {
	TotalInputTokens  int      `json:"total_input_tokens"`
	TotalOutputTokens int      `json:"total_output_tokens"`
	ContextWindowSize int      `json:"context_window_size"`
	UsedPercentage    *float64 `json:"used_percentage"`
}

// statusLineUpdateMsg carries the output from a status line command execution.
type statusLineUpdateMsg struct {
	Text string
}

// runStatusLineCmd runs the user's status line command, piping JSON session data
// to stdin, and returns the trimmed stdout. Returns empty string on error or timeout.
func runStatusLineCmd(cfg *config.StatusLineConfig, data statusLineData, timeout time.Duration) string {
	if cfg == nil || cfg.Type != "command" || cfg.Command == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Expand ~ in command path.
	command := cfg.Command
	if strings.HasPrefix(command, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			command = home + command[1:]
		}
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	// Use a process group so the timeout kills all child processes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Kill the entire process group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	// Pipe JSON data to stdin.
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	cmd.Stdin = bytes.NewReader(jsonData)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil // discard stderr

	if err := cmd.Run(); err != nil {
		return ""
	}

	// Process output: trim, keep non-empty lines.
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	var result []string
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return strings.Join(result, "\n")
}

// buildStatusLineData assembles the JSON payload from current TUI state.
func (m model) buildStatusLineData() statusLineData {
	cwd, _ := os.Getwd()

	var usedPct *float64
	total := m.tokens.TotalInputTokens + m.tokens.TotalCacheRead + m.tokens.TotalCacheWrite
	if total > 0 {
		// Default context window size is 200k.
		ctxSize := 200000
		pct := float64(total) / float64(ctxSize) * 100
		usedPct = &pct
	}

	displayName := m.modelName
	// Use the api package's display name if available, but avoid an import
	// cycle by using a simple map here for common models.
	nameMap := map[string]string{
		"claude-opus-4-6":    "Opus",
		"claude-sonnet-4-6":  "Sonnet",
		"claude-haiku-3-5":   "Haiku",
	}
	if dn, ok := nameMap[m.modelName]; ok {
		displayName = dn
	}

	// Use the full model ID from the API response (includes version date)
	// when available; fall back to the configured model name.
	modelID := m.modelName
	if m.resolvedModelID != "" {
		modelID = m.resolvedModelID
	}

	sessionID := ""
	if m.session != nil {
		sessionID = m.session.ID
	}

	return statusLineData{
		Cwd:       cwd,
		SessionID: sessionID,
		Model: statusLineModel{
			ID:          modelID,
			DisplayName: displayName,
		},
		Workspace: statusLineWorkspace{
			CurrentDir: cwd,
			ProjectDir: cwd,
		},
		Version:     m.version,
		OutputStyle: statusLineStyle{Name: "default"},
		Cost: statusLineCost{
			TotalCostUSD: m.tokens.TotalCostUSD,
		},
		ContextWindow: statusLineContext{
			TotalInputTokens:  m.tokens.TotalInputTokens,
			TotalOutputTokens: m.tokens.TotalOutputTokens,
			ContextWindowSize: 200000,
			UsedPercentage:    usedPct,
		},
	}
}

// refreshStatusLine returns a tea.Cmd that runs the status line command in
// the background and sends the result as a message.
func (m model) refreshStatusLine() tea.Cmd {
	if m.settings == nil || m.settings.StatusLine == nil {
		return nil
	}
	cfg := m.settings.StatusLine
	data := m.buildStatusLineData()
	return func() tea.Msg {
		text := runStatusLineCmd(cfg, data, 5*time.Second)
		return statusLineUpdateMsg{Text: text}
	}
}
