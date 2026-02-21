package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileWriteInput is the input schema for the FileWrite tool.
type FileWriteInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// FileWriteTool creates or overwrites files.
type FileWriteTool struct{}

// NewFileWriteTool creates a new FileWrite tool.
func NewFileWriteTool() *FileWriteTool {
	return &FileWriteTool{}
}

func (t *FileWriteTool) Name() string { return "FileWrite" }

func (t *FileWriteTool) Description() string {
	return `Creates or overwrites a file with the given content. The file_path must be an absolute path. Parent directories are created if they don't exist.`
}

func (t *FileWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {
      "type": "string",
      "description": "The absolute path to the file to write (must be absolute, not relative)"
    },
    "content": {
      "type": "string",
      "description": "The content to write to the file"
    }
  },
  "required": ["file_path", "content"],
  "additionalProperties": false
}`)
}

func (t *FileWriteTool) RequiresPermission(_ json.RawMessage) bool {
	return true
}

func (t *FileWriteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in FileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing FileWrite input: %w", err)
	}

	if in.FilePath == "" {
		return "Error: file_path is required", nil
	}

	if !filepath.IsAbs(in.FilePath) {
		return "Error: file_path must be an absolute path", nil
	}

	// Create parent directories.
	dir := filepath.Dir(in.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("Error creating directories: %v", err), nil
	}

	if err := os.WriteFile(in.FilePath, []byte(in.Content), 0644); err != nil {
		return fmt.Sprintf("Error writing file: %v", err), nil
	}

	return fmt.Sprintf("Successfully wrote to %s (%d bytes).", in.FilePath, len(in.Content)), nil
}
