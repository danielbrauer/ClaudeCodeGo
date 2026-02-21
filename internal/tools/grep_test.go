package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepTool_BasicSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n\nfunc hello() {\n\treturn\n}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "other.txt"), []byte("no match here\n"), 0644)

	tool := NewGrepTool(dir)
	input := buildGrepInput(t, map[string]interface{}{
		"pattern": "func hello",
		"path":    dir,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "test.go") {
		t.Errorf("expected test.go in results, got:\n%s", result)
	}
}

func TestGrepTool_ContentMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("line1\nline2\ntarget\nline4\n"), 0644)

	tool := NewGrepTool(dir)
	input := buildGrepInput(t, map[string]interface{}{
		"pattern":     "target",
		"path":        dir,
		"output_mode": "content",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "target") {
		t.Errorf("expected 'target' in content output, got:\n%s", result)
	}
}

func TestGrepTool_NoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("nothing here\n"), 0644)

	tool := NewGrepTool(dir)
	input := buildGrepInput(t, map[string]interface{}{
		"pattern": "nonexistent_pattern_xyz",
		"path":    dir,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No matches") {
		t.Errorf("expected 'No matches' message, got %q", result)
	}
}

func TestGrepTool_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("Hello World\n"), 0644)

	tool := NewGrepTool(dir)
	input := buildGrepInput(t, map[string]interface{}{
		"pattern": "hello world",
		"path":    dir,
		"-i":      true,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "No matches") {
		t.Errorf("should have found case-insensitive match, got:\n%s", result)
	}
}

func TestGrepTool_RequiresPermission(t *testing.T) {
	tool := NewGrepTool(".")
	if tool.RequiresPermission(nil) {
		t.Error("Grep should not require permission (read-only)")
	}
}

func TestApplyOffsetLimit(t *testing.T) {
	input := "line0\nline1\nline2\nline3\nline4"

	tests := []struct {
		name      string
		offset    *int
		limit     *int
		wantLines int
	}{
		{"no offset/limit", nil, nil, 5},
		{"offset 2", intPtr(2), nil, 3},
		{"limit 2", nil, intPtr(2), 2},
		{"offset 1, limit 2", intPtr(1), intPtr(2), 2},
		{"offset beyond end", intPtr(10), nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyOffsetLimit(input, tt.offset, tt.limit)
			if tt.wantLines == 0 {
				if result != "" {
					t.Errorf("expected empty, got %q", result)
				}
				return
			}
			lines := strings.Split(result, "\n")
			if len(lines) != tt.wantLines {
				t.Errorf("expected %d lines, got %d: %q", tt.wantLines, len(lines), result)
			}
		})
	}
}

func intPtr(n int) *int { return &n }

// buildGrepInput creates JSON input from a map, handling dash-prefixed keys.
func buildGrepInput(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal grep input: %v", err)
	}
	return json.RawMessage(data)
}
