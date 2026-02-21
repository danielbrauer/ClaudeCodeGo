package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	fileReadDefaultLimit = 2000
	fileReadMaxLineLen   = 2000
)

// FileReadInput is the input schema for the FileRead tool.
type FileReadInput struct {
	FilePath string `json:"file_path"`
	Offset   *int   `json:"offset,omitempty"` // 1-based line number
	Limit    *int   `json:"limit,omitempty"`
	Pages    string `json:"pages,omitempty"` // PDF page range (not implemented yet)
}

// FileReadTool reads files from the local filesystem.
type FileReadTool struct{}

// NewFileReadTool creates a new FileRead tool.
func NewFileReadTool() *FileReadTool {
	return &FileReadTool{}
}

func (t *FileReadTool) Name() string { return "FileRead" }

func (t *FileReadTool) Description() string {
	return `Reads a file from the local filesystem. The file_path parameter must be an absolute path. By default reads up to 2000 lines from the beginning. Use offset and limit for large files. Results are returned with line numbers (cat -n format).`
}

func (t *FileReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {
      "type": "string",
      "description": "The absolute path to the file to read"
    },
    "offset": {
      "type": "number",
      "description": "The line number to start reading from (1-based). Only provide if the file is too large to read at once"
    },
    "limit": {
      "type": "number",
      "description": "The number of lines to read. Only provide if the file is too large to read at once."
    },
    "pages": {
      "type": "string",
      "description": "Page range for PDF files (e.g., \"1-5\"). Only applicable to PDF files."
    }
  },
  "required": ["file_path"],
  "additionalProperties": false
}`)
}

func (t *FileReadTool) RequiresPermission(_ json.RawMessage) bool {
	return false // Read-only, no permission needed.
}

func (t *FileReadTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in FileReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing FileRead input: %w", err)
	}

	if in.FilePath == "" {
		return "Error: file_path is required", nil
	}

	// Check if file exists.
	info, err := os.Stat(in.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file not found: %s", in.FilePath), nil
		}
		return fmt.Sprintf("Error: %v", err), nil
	}

	if info.IsDir() {
		return fmt.Sprintf("Error: %s is a directory, not a file. Use ls via Bash to list directory contents.", in.FilePath), nil
	}

	f, err := os.Open(in.FilePath)
	if err != nil {
		return fmt.Sprintf("Error opening file: %v", err), nil
	}
	defer f.Close()

	// Determine offset and limit.
	offset := 1 // 1-based
	if in.Offset != nil && *in.Offset > 0 {
		offset = *in.Offset
	}

	limit := fileReadDefaultLimit
	if in.Limit != nil && *in.Limit > 0 {
		limit = *in.Limit
	}

	scanner := bufio.NewScanner(f)
	// Allow long lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var result strings.Builder
	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if linesRead >= limit {
			break
		}

		line := scanner.Text()
		// Truncate long lines.
		if len(line) > fileReadMaxLineLen {
			line = line[:fileReadMaxLineLen]
		}

		// cat -n format: right-aligned line number + tab + content
		fmt.Fprintf(&result, "%6d\t%s\n", lineNum, line)
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("Error reading file: %v", err), nil
	}

	output := result.String()
	if output == "" {
		if lineNum == 0 {
			return "(empty file)", nil
		}
		return fmt.Sprintf("(no lines in range: offset=%d, total lines=%d)", offset, lineNum), nil
	}

	return output, nil
}
