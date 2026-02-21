package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileEditTool_BasicEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world\nfoo bar\n"), 0644)

	tool := NewFileEditTool()
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "hello world",
		NewString: "hi there",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully edited") {
		t.Errorf("expected success message, got %q", result)
	}

	// Verify file contents.
	data, _ := os.ReadFile(path)
	if string(data) != "hi there\nfoo bar\n" {
		t.Errorf("file contents wrong: %q", string(data))
	}
}

func TestFileEditTool_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("aaa bbb aaa ccc aaa\n"), 0644)

	tool := NewFileEditTool()
	input, _ := json.Marshal(FileEditInput{
		FilePath:   path,
		OldString:  "aaa",
		NewString:  "xxx",
		ReplaceAll: true,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "3 occurrences") {
		t.Errorf("expected 3 occurrences message, got %q", result)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "xxx bbb xxx ccc xxx\n" {
		t.Errorf("file contents wrong: %q", string(data))
	}
}

func TestFileEditTool_OldStringNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello\n"), 0644)

	tool := NewFileEditTool()
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "missing",
		NewString: "replacement",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found' message, got %q", result)
	}
}

func TestFileEditTool_NotUnique(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("abc def abc\n"), 0644)

	tool := NewFileEditTool()
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "abc",
		NewString: "xyz",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "2 times") {
		t.Errorf("expected 'appears 2 times' message, got %q", result)
	}
}

func TestFileEditTool_SameStrings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello\n"), 0644)

	tool := NewFileEditTool()
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "hello",
		NewString: "hello",
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "must be different") {
		t.Errorf("expected 'must be different' message, got %q", result)
	}
}

func TestFileEditTool_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sh")
	os.WriteFile(path, []byte("#!/bin/bash\necho hello\n"), 0755)

	tool := NewFileEditTool()
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "echo hello",
		NewString: "echo world",
	})
	_, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0755 {
		t.Errorf("expected permissions 0755, got %o", info.Mode().Perm())
	}
}

func TestFileEditTool_RequiresPermission(t *testing.T) {
	tool := NewFileEditTool()
	if !tool.RequiresPermission(nil) {
		t.Error("FileEdit should require permission")
	}
}
