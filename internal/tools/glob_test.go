package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobTool_BasicPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0644)

	tool := NewGlobTool(dir)
	input, _ := json.Marshal(GlobInput{Pattern: "*.go"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "a.go") || !strings.Contains(result, "b.go") {
		t.Errorf("expected both .go files, got:\n%s", result)
	}
	if strings.Contains(result, "c.txt") {
		t.Errorf("should not match .txt file, got:\n%s", result)
	}
}

func TestGlobTool_RecursivePattern(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(dir, "top.go"), []byte("package top"), 0644)
	os.WriteFile(filepath.Join(subDir, "nested.go"), []byte("package nested"), 0644)

	tool := NewGlobTool(dir)
	input, _ := json.Marshal(GlobInput{Pattern: "**/*.go"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "nested.go") {
		t.Errorf("expected nested.go in recursive glob, got:\n%s", result)
	}
}

func TestGlobTool_NoMatches(t *testing.T) {
	dir := t.TempDir()

	tool := NewGlobTool(dir)
	input, _ := json.Marshal(GlobInput{Pattern: "*.xyz"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No files matched") {
		t.Errorf("expected 'No files matched' message, got %q", result)
	}
}

func TestGlobTool_WithPath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "src")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "main.go"), []byte("package main"), 0644)

	tool := NewGlobTool(dir)
	input, _ := json.Marshal(GlobInput{
		Pattern: "*.go",
		Path:    subDir,
	})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "main.go") {
		t.Errorf("expected main.go, got:\n%s", result)
	}
}

func TestGlobTool_RequiresPermission(t *testing.T) {
	tool := NewGlobTool(".")
	if tool.RequiresPermission(nil) {
		t.Error("Glob should not require permission (read-only)")
	}
}
