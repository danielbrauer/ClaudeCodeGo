package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBashTool_SimpleCommand(t *testing.T) {
	tool := NewBashTool(t.TempDir())

	input, _ := json.Marshal(BashInput{Command: "echo hello"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(result) != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestBashTool_EmptyCommand(t *testing.T) {
	tool := NewBashTool(t.TempDir())

	input, _ := json.Marshal(BashInput{Command: ""})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "command is required") {
		t.Errorf("expected error about empty command, got %q", result)
	}
}

func TestBashTool_ExitCode(t *testing.T) {
	tool := NewBashTool(t.TempDir())

	input, _ := json.Marshal(BashInput{Command: "exit 42"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Exit code: 42") {
		t.Errorf("expected exit code 42 in result, got %q", result)
	}
}

func TestBashTool_Stderr(t *testing.T) {
	tool := NewBashTool(t.TempDir())

	input, _ := json.Marshal(BashInput{Command: "echo error >&2"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "error") {
		t.Errorf("expected stderr output in result, got %q", result)
	}
}

func TestBashTool_Timeout(t *testing.T) {
	tool := NewBashTool(t.TempDir())

	timeout := 100 // 100ms
	input, _ := json.Marshal(BashInput{
		Command: "sleep 10",
		Timeout: &timeout,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "timed out") {
		t.Errorf("expected timeout message, got %q", result)
	}
}

func TestBashTool_WorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	tool := NewBashTool(dir)

	input, _ := json.Marshal(BashInput{Command: "pwd"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(result) != dir {
		t.Errorf("expected working dir %q, got %q", dir, strings.TrimSpace(result))
	}
}

func TestBashTool_RequiresPermission(t *testing.T) {
	tool := NewBashTool(t.TempDir())
	if !tool.RequiresPermission(nil) {
		t.Error("Bash tool should require permission")
	}
}

func TestBashTool_ContextCancellation(t *testing.T) {
	tool := NewBashTool(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	input, _ := json.Marshal(BashInput{Command: "sleep 10"})
	_, err := tool.Execute(ctx, input)
	// Should either error or return quickly.
	if err == nil {
		// If no error, the result should indicate something went wrong.
		t.Log("no error on cancelled context (command may not have started)")
	}
}
