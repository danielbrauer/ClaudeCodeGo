package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// mockTool is a simple tool for testing the registry.
type mockTool struct {
	name           string
	needsPermission bool
	result         string
	err            error
}

func (t *mockTool) Name() string                          { return t.name }
func (t *mockTool) Description() string                   { return "mock tool" }
func (t *mockTool) InputSchema() json.RawMessage          { return json.RawMessage(`{"type":"object"}`) }
func (t *mockTool) RequiresPermission(_ json.RawMessage) bool { return t.needsPermission }
func (t *mockTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return t.result, t.err
}

// mockPermission records permission requests.
type mockPermission struct {
	allow    bool
	requests []string
}

func (p *mockPermission) RequestPermission(_ context.Context, toolName string, _ json.RawMessage) (bool, error) {
	p.requests = append(p.requests, toolName)
	return p.allow, nil
}

func TestRegistry_RegisterAndHasTool(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&mockTool{name: "TestTool", result: "ok"})

	if !r.HasTool("TestTool") {
		t.Error("expected HasTool(TestTool) to return true")
	}
	if r.HasTool("NonExistent") {
		t.Error("expected HasTool(NonExistent) to return false")
	}
}

func TestRegistry_Execute(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&mockTool{name: "Echo", result: "hello"})

	result, err := r.Execute(context.Background(), "Echo", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestRegistry_ExecuteUnknownTool(t *testing.T) {
	r := NewRegistry(nil)

	_, err := r.Execute(context.Background(), "Missing", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestRegistry_PermissionDenied(t *testing.T) {
	perm := &mockPermission{allow: false}
	r := NewRegistry(perm)
	r.Register(&mockTool{name: "Dangerous", needsPermission: true, result: "done"})

	_, err := r.Execute(context.Background(), "Dangerous", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error when permission denied")
	}
	if len(perm.requests) != 1 || perm.requests[0] != "Dangerous" {
		t.Errorf("expected one permission request for Dangerous, got %v", perm.requests)
	}
}

func TestRegistry_PermissionAllowed(t *testing.T) {
	perm := &mockPermission{allow: true}
	r := NewRegistry(perm)
	r.Register(&mockTool{name: "Dangerous", needsPermission: true, result: "done"})

	result, err := r.Execute(context.Background(), "Dangerous", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "done" {
		t.Errorf("expected 'done', got %q", result)
	}
}

func TestRegistry_NoPermissionNeeded(t *testing.T) {
	perm := &mockPermission{allow: false} // would deny if asked
	r := NewRegistry(perm)
	r.Register(&mockTool{name: "ReadOnly", needsPermission: false, result: "data"})

	result, err := r.Execute(context.Background(), "ReadOnly", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "data" {
		t.Errorf("expected 'data', got %q", result)
	}
	if len(perm.requests) != 0 {
		t.Error("should not have requested permission for read-only tool")
	}
}

func TestRegistry_Definitions(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&mockTool{name: "A", result: "a"})
	r.Register(&mockTool{name: "B", result: "b"})
	r.Register(&mockTool{name: "C", result: "c"})

	defs := r.Definitions()
	if len(defs) != 3 {
		t.Fatalf("expected 3 definitions, got %d", len(defs))
	}
	// Should preserve registration order.
	if defs[0].Name != "A" || defs[1].Name != "B" || defs[2].Name != "C" {
		t.Errorf("definitions not in registration order: %v, %v, %v", defs[0].Name, defs[1].Name, defs[2].Name)
	}
}
