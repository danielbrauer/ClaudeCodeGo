package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FileEditInput is the input schema for the FileEdit tool.
type FileEditInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// FileEditTool performs exact string replacements in files.
type FileEditTool struct{}

// NewFileEditTool creates a new FileEdit tool.
func NewFileEditTool() *FileEditTool {
	return &FileEditTool{}
}

func (t *FileEditTool) Name() string { return "FileEdit" }

func (t *FileEditTool) Description() string {
	return `Performs exact string replacements in files. The old_string must be unique in the file unless replace_all is true. The new_string must be different from old_string. Use this tool for making targeted edits to existing files.`
}

func (t *FileEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {
      "type": "string",
      "description": "The absolute path to the file to modify"
    },
    "old_string": {
      "type": "string",
      "description": "The text to replace"
    },
    "new_string": {
      "type": "string",
      "description": "The text to replace it with (must be different from old_string)"
    },
    "replace_all": {
      "type": "boolean",
      "description": "Replace all occurrences of old_string (default false)",
      "default": false
    }
  },
  "required": ["file_path", "old_string", "new_string"],
  "additionalProperties": false
}`)
}

func (t *FileEditTool) RequiresPermission(_ json.RawMessage) bool {
	return true
}

func (t *FileEditTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in FileEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing FileEdit input: %w", err)
	}

	if in.FilePath == "" {
		return "Error: file_path is required", nil
	}
	if in.OldString == in.NewString {
		return "Error: new_string must be different from old_string", nil
	}

	// Read the file.
	data, err := os.ReadFile(in.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file not found: %s", in.FilePath), nil
		}
		return fmt.Sprintf("Error reading file: %v", err), nil
	}

	content := string(data)

	// Check that old_string exists.
	count := strings.Count(content, in.OldString)
	if count == 0 {
		return fmt.Sprintf("Error: old_string not found in %s. Make sure the string matches exactly, including whitespace and indentation.", in.FilePath), nil
	}

	// If not replace_all, verify uniqueness.
	if !in.ReplaceAll && count > 1 {
		return fmt.Sprintf("Error: old_string appears %d times in %s. Use replace_all=true to replace all occurrences, or provide more surrounding context to make it unique.", count, in.FilePath), nil
	}

	// Perform replacement.
	var newContent string
	if in.ReplaceAll {
		newContent = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		newContent = strings.Replace(content, in.OldString, in.NewString, 1)
	}

	// Preserve original file permissions.
	info, err := os.Stat(in.FilePath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	if err := os.WriteFile(in.FilePath, []byte(newContent), info.Mode().Perm()); err != nil {
		return fmt.Sprintf("Error writing file: %v", err), nil
	}

	if in.ReplaceAll {
		return fmt.Sprintf("Replaced %d occurrences in %s.", count, in.FilePath), nil
	}
	return fmt.Sprintf("Successfully edited %s.", in.FilePath), nil
}
