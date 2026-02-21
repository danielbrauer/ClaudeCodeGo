package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileReadTool_BasicRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line 1\nline 2\nline 3\n"), 0644)

	tool := NewFileReadTool()
	input, _ := json.Marshal(FileReadInput{FilePath: path})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain line numbers in cat -n format.
	if !strings.Contains(result, "1\tline 1") {
		t.Errorf("expected cat -n format with line 1, got:\n%s", result)
	}
	if !strings.Contains(result, "2\tline 2") {
		t.Errorf("expected cat -n format with line 2, got:\n%s", result)
	}
	if !strings.Contains(result, "3\tline 3") {
		t.Errorf("expected cat -n format with line 3, got:\n%s", result)
	}
}

func TestFileReadTool_OffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line "+string(rune('A'-1+i)))
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	tool := NewFileReadTool()
	offset := 5
	limit := 3
	input, _ := json.Marshal(FileReadInput{
		FilePath: path,
		Offset:   &offset,
		Limit:    &limit,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultLines := strings.Split(strings.TrimSpace(result), "\n")
	if len(resultLines) != 3 {
		t.Errorf("expected 3 lines, got %d:\n%s", len(resultLines), result)
	}
	// First line should be line 5.
	if !strings.Contains(resultLines[0], "5\t") {
		t.Errorf("expected line number 5, got: %s", resultLines[0])
	}
}

func TestFileReadTool_FileNotFound(t *testing.T) {
	tool := NewFileReadTool()
	input, _ := json.Marshal(FileReadInput{FilePath: "/nonexistent/file.txt"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found' message, got %q", result)
	}
}

func TestFileReadTool_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte(""), 0644)

	tool := NewFileReadTool()
	input, _ := json.Marshal(FileReadInput{FilePath: path})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "empty") {
		t.Errorf("expected empty file message, got %q", result)
	}
}

func TestFileReadTool_Directory(t *testing.T) {
	tool := NewFileReadTool()
	input, _ := json.Marshal(FileReadInput{FilePath: t.TempDir()})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "directory") {
		t.Errorf("expected directory error message, got %q", result)
	}
}

func TestFileReadTool_RequiresPermission(t *testing.T) {
	tool := NewFileReadTool()
	if tool.RequiresPermission(nil) {
		t.Error("FileRead should not require permission (read-only)")
	}
}
