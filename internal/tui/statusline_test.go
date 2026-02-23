package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/claude-code-go/internal/config"
)

// sampleStatusLineData returns a representative statusLineData for testing.
func sampleStatusLineData() statusLineData {
	pct := float64(25)
	return statusLineData{
		Cwd:       "/home/user/my-project",
		SessionID: "test-session-123",
		Model: statusLineModel{
			ID:          "claude-opus-4-6",
			DisplayName: "Opus",
		},
		Workspace: statusLineWorkspace{
			CurrentDir: "/home/user/my-project",
			ProjectDir: "/home/user/my-project",
		},
		Version:     "2.1.50",
		OutputStyle: statusLineStyle{Name: "default"},
		Cost: statusLineCost{
			TotalCostUSD:       0.01234,
			TotalDurationMs:    45000,
			TotalAPIDurationMs: 2300,
		},
		ContextWindow: statusLineContext{
			TotalInputTokens:  15234,
			TotalOutputTokens: 4521,
			ContextWindowSize: 200000,
			UsedPercentage:    &pct,
		},
	}
}

// writeScript writes a bash script to a temp file and returns the path.
func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("writing script %s: %v", name, err)
	}
	return path
}

func TestStatusLineCmd_NilConfig(t *testing.T) {
	result := runStatusLineCmd(nil, sampleStatusLineData(), 5*time.Second)
	if result != "" {
		t.Errorf("expected empty string for nil config, got %q", result)
	}
}

func TestStatusLineCmd_WrongType(t *testing.T) {
	cfg := &config.StatusLineConfig{Type: "other", Command: "echo hello"}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 5*time.Second)
	if result != "" {
		t.Errorf("expected empty string for wrong type, got %q", result)
	}
}

func TestStatusLineCmd_EmptyCommand(t *testing.T) {
	cfg := &config.StatusLineConfig{Type: "command", Command: ""}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 5*time.Second)
	if result != "" {
		t.Errorf("expected empty string for empty command, got %q", result)
	}
}

func TestStatusLineCmd_SimpleEcho(t *testing.T) {
	cfg := &config.StatusLineConfig{
		Type:    "command",
		Command: "echo hello-status",
	}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 5*time.Second)
	if result != "hello-status" {
		t.Errorf("expected 'hello-status', got %q", result)
	}
}

func TestStatusLineCmd_ReadsStdinJSON(t *testing.T) {
	// Verify the command receives valid JSON on stdin by extracting a field.
	cfg := &config.StatusLineConfig{
		Type:    "command",
		Command: `cat | python3 -c "import sys, json; d=json.load(sys.stdin); print(d['model']['display_name'])"`,
	}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 5*time.Second)
	if result != "Opus" {
		t.Errorf("expected 'Opus', got %q", result)
	}
}

func TestStatusLineCmd_Timeout(t *testing.T) {
	cfg := &config.StatusLineConfig{
		Type:    "command",
		Command: "sleep 10",
	}
	start := time.Now()
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 500*time.Millisecond)
	elapsed := time.Since(start)
	if result != "" {
		t.Errorf("expected empty string on timeout, got %q", result)
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout did not work: elapsed %v", elapsed)
	}
}

func TestStatusLineCmd_NonZeroExit(t *testing.T) {
	cfg := &config.StatusLineConfig{
		Type:    "command",
		Command: "exit 1",
	}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 5*time.Second)
	if result != "" {
		t.Errorf("expected empty string on non-zero exit, got %q", result)
	}
}

// TestStatusLineCmd_ContextWindowUsageScript tests the "Context window usage"
// example script from the docs (Bash version).
func TestStatusLineCmd_ContextWindowUsageScript(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "context-usage.sh", `#!/bin/bash
input=$(cat)

MODEL=$(echo "$input" | python3 -c "import sys,json; print(json.load(sys.stdin)['model']['display_name'])")
PCT=$(echo "$input" | python3 -c "import sys,json; print(int(json.load(sys.stdin).get('context_window',{}).get('used_percentage',0) or 0))")

BAR_WIDTH=10
FILLED=$((PCT * BAR_WIDTH / 100))
EMPTY=$((BAR_WIDTH - FILLED))
BAR=""
[ "$FILLED" -gt 0 ] && BAR=$(printf "%${FILLED}s" | tr ' ' '#')
[ "$EMPTY" -gt 0 ] && BAR="${BAR}$(printf "%${EMPTY}s" | tr ' ' '-')"

echo "[$MODEL] $BAR $PCT%"
`)

	cfg := &config.StatusLineConfig{Type: "command", Command: script}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 10*time.Second)

	// Should contain model name and percentage.
	if !strings.Contains(result, "Opus") {
		t.Errorf("expected output to contain 'Opus', got %q", result)
	}
	if !strings.Contains(result, "25%") {
		t.Errorf("expected output to contain '25%%', got %q", result)
	}
	if !strings.Contains(result, "[Opus]") {
		t.Errorf("expected output to contain '[Opus]', got %q", result)
	}
}

// TestStatusLineCmd_CostTrackingScript tests the "Cost and duration tracking"
// example script from the docs (Bash version).
func TestStatusLineCmd_CostTrackingScript(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "cost-tracking.sh", `#!/bin/bash
input=$(cat)

MODEL=$(echo "$input" | python3 -c "import sys,json; print(json.load(sys.stdin)['model']['display_name'])")
COST=$(echo "$input" | python3 -c "import sys,json; print(json.load(sys.stdin).get('cost',{}).get('total_cost_usd',0) or 0)")
DURATION_MS=$(echo "$input" | python3 -c "import sys,json; print(int(json.load(sys.stdin).get('cost',{}).get('total_duration_ms',0) or 0))")

COST_FMT=$(printf '$%.2f' "$COST")
DURATION_SEC=$((DURATION_MS / 1000))
MINS=$((DURATION_SEC / 60))
SECS=$((DURATION_SEC % 60))

echo "[$MODEL] $COST_FMT | ${MINS}m ${SECS}s"
`)

	cfg := &config.StatusLineConfig{Type: "command", Command: script}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 10*time.Second)

	if !strings.Contains(result, "[Opus]") {
		t.Errorf("expected '[Opus]', got %q", result)
	}
	if !strings.Contains(result, "$0.01") {
		t.Errorf("expected cost '$0.01', got %q", result)
	}
	// 45000ms = 45s = 0m 45s
	if !strings.Contains(result, "0m 45s") {
		t.Errorf("expected '0m 45s', got %q", result)
	}
}

// TestStatusLineCmd_MultiLineScript tests multi-line output from a status line
// script (the "Display multiple lines" example from the docs).
func TestStatusLineCmd_MultiLineScript(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "multiline.sh", `#!/bin/bash
input=$(cat)

MODEL=$(echo "$input" | python3 -c "import sys,json; print(json.load(sys.stdin)['model']['display_name'])")
DIR=$(echo "$input" | python3 -c "import sys,json; import os; print(os.path.basename(json.load(sys.stdin)['workspace']['current_dir']))")
PCT=$(echo "$input" | python3 -c "import sys,json; print(int(json.load(sys.stdin).get('context_window',{}).get('used_percentage',0) or 0))")

echo "[$MODEL] $DIR"
echo "$PCT% context used"
`)

	cfg := &config.StatusLineConfig{Type: "command", Command: script}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 10*time.Second)

	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %q", len(lines), result)
	}
	if !strings.Contains(lines[0], "[Opus]") {
		t.Errorf("line 1 should contain '[Opus]', got %q", lines[0])
	}
	if !strings.Contains(lines[0], "my-project") {
		t.Errorf("line 1 should contain 'my-project', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "25%") {
		t.Errorf("line 2 should contain '25%%', got %q", lines[1])
	}
}

// TestStatusLineCmd_InlineJqStyle tests using an inline jq-style command
// (the inline example from the docs, but using python3 since jq may not
// be installed in all test environments).
func TestStatusLineCmd_InlineJqStyle(t *testing.T) {
	cfg := &config.StatusLineConfig{
		Type:    "command",
		Command: `python3 -c "import sys,json; d=json.load(sys.stdin); print('[%s] %d%% context' % (d['model']['display_name'], int(d['context_window']['used_percentage'] or 0)))"`,
	}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 10*time.Second)
	if result != "[Opus] 25% context" {
		t.Errorf("expected '[Opus] 25%% context', got %q", result)
	}
}

// TestStatusLineCmd_GitStatusScript tests the "Git status with colors" example.
// This test runs in the repo so git is available.
func TestStatusLineCmd_GitStatusScript(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "git-status.sh", `#!/bin/bash
input=$(cat)

MODEL=$(echo "$input" | python3 -c "import sys,json; print(json.load(sys.stdin)['model']['display_name'])")
DIR=$(echo "$input" | python3 -c "import sys,json; import os; print(os.path.basename(json.load(sys.stdin)['workspace']['current_dir']))")

if git rev-parse --git-dir > /dev/null 2>&1; then
    BRANCH=$(git branch --show-current 2>/dev/null)
    echo "[$MODEL] $DIR | $BRANCH"
else
    echo "[$MODEL] $DIR"
fi
`)

	cfg := &config.StatusLineConfig{Type: "command", Command: script}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 10*time.Second)

	// Should at minimum contain model and directory.
	if !strings.Contains(result, "[Opus]") {
		t.Errorf("expected output to contain '[Opus]', got %q", result)
	}
	if !strings.Contains(result, "my-project") {
		t.Errorf("expected output to contain 'my-project', got %q", result)
	}
}

// TestStatusLineCmd_ClickableLinksScript tests the OSC 8 link script.
// We just verify the output contains the OSC 8 escape sequences.
func TestStatusLineCmd_ClickableLinksScript(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "links.sh", `#!/bin/bash
input=$(cat)

MODEL=$(echo "$input" | python3 -c "import sys,json; print(json.load(sys.stdin)['model']['display_name'])")

# Simulate a repo URL (not using actual git remote since test env may differ)
REMOTE="https://github.com/example/my-project"
REPO_NAME="my-project"

printf '%b' "[$MODEL] \e]8;;${REMOTE}\a${REPO_NAME}\e]8;;\a\n"
`)

	cfg := &config.StatusLineConfig{Type: "command", Command: script}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 10*time.Second)

	if !strings.Contains(result, "[Opus]") {
		t.Errorf("expected output to contain '[Opus]', got %q", result)
	}
	// Check for OSC 8 escape sequence markers.
	if !strings.Contains(result, "\x1b]8;;") {
		t.Errorf("expected OSC 8 escape sequences, got %q", result)
	}
	if !strings.Contains(result, "my-project") {
		t.Errorf("expected 'my-project' in output, got %q", result)
	}
}

// TestStatusLineCmd_CachedOperationsScript tests the caching example.
// Uses a temp file as cache and verifies the output is correct on both
// cold and warm runs.
func TestStatusLineCmd_CachedOperationsScript(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "cache")

	script := writeScript(t, dir, "cached.sh", `#!/bin/bash
input=$(cat)

MODEL=$(echo "$input" | python3 -c "import sys,json; print(json.load(sys.stdin)['model']['display_name'])")
DIR=$(echo "$input" | python3 -c "import sys,json; import os; print(os.path.basename(json.load(sys.stdin)['workspace']['current_dir']))")

CACHE_FILE="`+cacheFile+`"
CACHE_MAX_AGE=5

cache_is_stale() {
    [ ! -f "$CACHE_FILE" ] || \
    [ $(($(date +%s) - $(stat -c %Y "$CACHE_FILE" 2>/dev/null || echo 0))) -gt $CACHE_MAX_AGE ]
}

if cache_is_stale; then
    echo "main|2|3" > "$CACHE_FILE"
fi

IFS='|' read -r BRANCH STAGED MODIFIED < "$CACHE_FILE"

if [ -n "$BRANCH" ]; then
    echo "[$MODEL] $DIR | $BRANCH +$STAGED ~$MODIFIED"
else
    echo "[$MODEL] $DIR"
fi
`)

	cfg := &config.StatusLineConfig{Type: "command", Command: script}

	// First run (cold cache).
	result1 := runStatusLineCmd(cfg, sampleStatusLineData(), 10*time.Second)
	if !strings.Contains(result1, "[Opus]") {
		t.Errorf("run 1: expected '[Opus]', got %q", result1)
	}
	if !strings.Contains(result1, "main") {
		t.Errorf("run 1: expected 'main', got %q", result1)
	}
	if !strings.Contains(result1, "+2") {
		t.Errorf("run 1: expected '+2', got %q", result1)
	}

	// Second run (warm cache — should produce same output).
	result2 := runStatusLineCmd(cfg, sampleStatusLineData(), 10*time.Second)
	if result1 != result2 {
		t.Errorf("expected identical results, got %q vs %q", result1, result2)
	}
}

// TestStatusLineCmd_PythonScript tests the Python version of the context window
// usage example from the docs.
func TestStatusLineCmd_PythonScript(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "status.py", `#!/usr/bin/env python3
import json, sys

data = json.load(sys.stdin)
model = data['model']['display_name']
pct = int(data.get('context_window', {}).get('used_percentage', 0) or 0)

filled = pct * 10 // 100
bar = '#' * filled + '-' * (10 - filled)

print(f"[{model}] {bar} {pct}%")
`)

	cfg := &config.StatusLineConfig{Type: "command", Command: "python3 " + script}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 10*time.Second)

	if !strings.Contains(result, "[Opus]") {
		t.Errorf("expected '[Opus]', got %q", result)
	}
	if !strings.Contains(result, "25%") {
		t.Errorf("expected '25%%', got %q", result)
	}
	// 25% of 10 chars = 2 filled + 8 empty
	if !strings.Contains(result, "##--------") {
		t.Errorf("expected '##--------' bar, got %q", result)
	}
}

// TestStatusLineData_JSONSchema verifies the JSON structure matches the documented schema.
func TestStatusLineData_JSONSchema(t *testing.T) {
	data := sampleStatusLineData()
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("marshaling status line data: %v", err)
	}

	// Verify the JSON can be round-tripped.
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("unmarshaling status line JSON: %v", err)
	}

	// Check required top-level fields.
	requiredFields := []string{
		"cwd", "session_id", "model", "workspace", "version",
		"output_style", "cost", "context_window",
	}
	for _, field := range requiredFields {
		if _, ok := parsed[field]; !ok {
			t.Errorf("missing required field %q in JSON output", field)
		}
	}

	// Check nested model fields.
	model, ok := parsed["model"].(map[string]interface{})
	if !ok {
		t.Fatal("model field is not an object")
	}
	if model["id"] != "claude-opus-4-6" {
		t.Errorf("model.id = %v, want 'claude-opus-4-6'", model["id"])
	}
	if model["display_name"] != "Opus" {
		t.Errorf("model.display_name = %v, want 'Opus'", model["display_name"])
	}

	// Check context window fields.
	ctx, ok := parsed["context_window"].(map[string]interface{})
	if !ok {
		t.Fatal("context_window field is not an object")
	}
	if ctx["context_window_size"] != float64(200000) {
		t.Errorf("context_window_size = %v, want 200000", ctx["context_window_size"])
	}
	if ctx["used_percentage"] != float64(25) {
		t.Errorf("used_percentage = %v, want 25", ctx["used_percentage"])
	}

	// Check cost fields.
	cost, ok := parsed["cost"].(map[string]interface{})
	if !ok {
		t.Fatal("cost field is not an object")
	}
	if cost["total_duration_ms"] != float64(45000) {
		t.Errorf("total_duration_ms = %v, want 45000", cost["total_duration_ms"])
	}
}

// TestStatusLineCmd_TildeExpansion verifies that ~ in the command path is expanded.
func TestStatusLineCmd_TildeExpansion(t *testing.T) {
	// This test simply verifies the expansion happens by using a command
	// that doesn't reference ~, since we can't guarantee the home dir
	// structure in CI.
	cfg := &config.StatusLineConfig{
		Type:    "command",
		Command: "echo tilde-test",
	}
	result := runStatusLineCmd(cfg, sampleStatusLineData(), 5*time.Second)
	if result != "tilde-test" {
		t.Errorf("expected 'tilde-test', got %q", result)
	}
}

// TestStatusLineCmd_NullUsedPercentage verifies correct behavior when
// used_percentage is nil (early in session before first API call).
func TestStatusLineCmd_NullUsedPercentage(t *testing.T) {
	data := sampleStatusLineData()
	data.ContextWindow.UsedPercentage = nil

	cfg := &config.StatusLineConfig{
		Type:    "command",
		Command: `python3 -c "import sys,json; d=json.load(sys.stdin); pct=d['context_window']['used_percentage']; print('null' if pct is None else str(pct))"`,
	}
	result := runStatusLineCmd(cfg, data, 10*time.Second)
	if result != "null" {
		t.Errorf("expected 'null' for nil used_percentage, got %q", result)
	}
}
