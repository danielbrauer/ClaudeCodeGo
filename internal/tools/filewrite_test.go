package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileWriteTool_CreateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	tool := NewFileWriteTool()
	input, _ := json.Marshal(FileWriteInput{
		FilePath: path,
		Content:  "hello world\n",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Errorf("expected success message, got %q", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("error reading file: %v", err)
	}
	if string(data) != "hello world\n" {
		t.Errorf("file contents wrong: %q", string(data))
	}
}

func TestFileWriteTool_CreateDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "file.txt")

	tool := NewFileWriteTool()
	input, _ := json.Marshal(FileWriteInput{
		FilePath: path,
		Content:  "nested\n",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Errorf("expected success message, got %q", result)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "nested\n" {
		t.Errorf("file contents wrong: %q", string(data))
	}
}

func TestFileWriteTool_OverwriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("old content"), 0644)

	tool := NewFileWriteTool()
	input, _ := json.Marshal(FileWriteInput{
		FilePath: path,
		Content:  "new content",
	})
	_, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new content" {
		t.Errorf("file should be overwritten, got %q", string(data))
	}
}

func TestFileWriteTool_RelativePath(t *testing.T) {
	tool := NewFileWriteTool()
	input, _ := json.Marshal(FileWriteInput{
		FilePath: "relative/path.txt",
		Content:  "content",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "absolute path") {
		t.Errorf("expected absolute path error, got %q", result)
	}
}

func TestFileWriteTool_RequiresPermission(t *testing.T) {
	tool := NewFileWriteTool()
	if !tool.RequiresPermission(nil) {
		t.Error("FileWrite should require permission")
	}
}
