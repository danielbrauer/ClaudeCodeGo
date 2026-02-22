package hooks

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRunPreToolUse_NoHooks(t *testing.T) {
	r := NewRunner(HookConfig{})
	err := r.RunPreToolUse(context.Background(), "Bash", json.RawMessage(`{"command":"ls"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunPreToolUse_SuccessfulCommand(t *testing.T) {
	r := NewRunner(HookConfig{
		PreToolUse: []HookDef{
			{Type: "command", Command: "true"},
		},
	})
	err := r.RunPreToolUse(context.Background(), "Bash", json.RawMessage(`{"command":"ls"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunPreToolUse_BlockingCommand(t *testing.T) {
	r := NewRunner(HookConfig{
		PreToolUse: []HookDef{
			{Type: "command", Command: "false"},
		},
	})
	err := r.RunPreToolUse(context.Background(), "Bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if err == nil {
		t.Fatal("expected error from blocking hook, got nil")
	}
}

func TestRunPostToolUse_NoHooks(t *testing.T) {
	r := NewRunner(HookConfig{})
	err := r.RunPostToolUse(context.Background(), "Bash", json.RawMessage(`{}`), "output", false)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunPostToolUse_WithCommand(t *testing.T) {
	r := NewRunner(HookConfig{
		PostToolUse: []HookDef{
			{Type: "command", Command: "true"},
		},
	})
	err := r.RunPostToolUse(context.Background(), "Bash", json.RawMessage(`{}`), "output", false)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunUserPromptSubmit_NoHooks(t *testing.T) {
	r := NewRunner(HookConfig{})
	result, err := r.RunUserPromptSubmit(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.Block {
		t.Fatal("expected Block=false")
	}
	if result.Message != "hello" {
		t.Fatalf("expected message 'hello', got %q", result.Message)
	}
}

func TestRunUserPromptSubmit_ModifiesMessage(t *testing.T) {
	r := NewRunner(HookConfig{
		UserPromptSubmit: []HookDef{
			{Type: "command", Command: "echo 'modified message'"},
		},
	})
	result, err := r.RunUserPromptSubmit(context.Background(), "original")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.Block {
		t.Fatal("expected Block=false")
	}
	if result.Message != "modified message" {
		t.Fatalf("expected 'modified message', got %q", result.Message)
	}
}

func TestRunUserPromptSubmit_BlocksOnFailure(t *testing.T) {
	r := NewRunner(HookConfig{
		UserPromptSubmit: []HookDef{
			{Type: "command", Command: "false"},
		},
	})
	result, err := r.RunUserPromptSubmit(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !result.Block {
		t.Fatal("expected Block=true")
	}
}

func TestRunSessionStart_NoHooks(t *testing.T) {
	r := NewRunner(HookConfig{})
	err := r.RunSessionStart(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunSessionStart_WithCommand(t *testing.T) {
	r := NewRunner(HookConfig{
		SessionStart: []HookDef{
			{Type: "command", Command: "true"},
		},
	})
	err := r.RunSessionStart(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunStop_NoHooks(t *testing.T) {
	r := NewRunner(HookConfig{})
	err := r.RunStop(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunStop_WithCommand(t *testing.T) {
	r := NewRunner(HookConfig{
		Stop: []HookDef{
			{Type: "command", Command: "true"},
		},
	})
	err := r.RunStop(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunPermissionRequest_NoHooks(t *testing.T) {
	r := NewRunner(HookConfig{})
	err := r.RunPermissionRequest(context.Background(), "Bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunPermissionRequest_WithCommand(t *testing.T) {
	r := NewRunner(HookConfig{
		PermissionRequest: []HookDef{
			{Type: "command", Command: "true"},
		},
	})
	err := r.RunPermissionRequest(context.Background(), "Bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPromptHook(t *testing.T) {
	r := NewRunner(HookConfig{
		PreToolUse: []HookDef{
			{Type: "prompt", Prompt: "Check for sensitive data"},
		},
	})
	err := r.RunPreToolUse(context.Background(), "Bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestUnknownHookType(t *testing.T) {
	r := NewRunner(HookConfig{
		PreToolUse: []HookDef{
			{Type: "unknown"},
		},
	})
	err := r.RunPreToolUse(context.Background(), "Bash", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown hook type, got nil")
	}
}

func TestEnvironmentVariablesPassed(t *testing.T) {
	r := NewRunner(HookConfig{
		PreToolUse: []HookDef{
			{Type: "command", Command: "test \"$TOOL_NAME\" = \"Bash\" && test \"$HOOK_EVENT\" = \"PreToolUse\""},
		},
	})
	err := r.RunPreToolUse(context.Background(), "Bash", json.RawMessage(`{"command":"ls"}`))
	if err != nil {
		t.Fatalf("environment variables not set correctly: %v", err)
	}
}

func TestMultipleHooks(t *testing.T) {
	r := NewRunner(HookConfig{
		PreToolUse: []HookDef{
			{Type: "command", Command: "true"},
			{Type: "command", Command: "true"},
		},
	})
	err := r.RunPreToolUse(context.Background(), "Bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestMultipleHooks_SecondFails(t *testing.T) {
	r := NewRunner(HookConfig{
		PreToolUse: []HookDef{
			{Type: "command", Command: "true"},
			{Type: "command", Command: "false"},
		},
	})
	err := r.RunPreToolUse(context.Background(), "Bash", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from second hook, got nil")
	}
}
