package tools

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	// Dispatch based on file extension.
	ext := strings.ToLower(filepath.Ext(in.FilePath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return t.readImage(in.FilePath, ext)
	case ".pdf":
		return t.readPDF(in.FilePath, in.Pages)
	case ".ipynb":
		return t.readNotebook(in.FilePath)
	}

	// Default: read as text file.
	return t.readTextFile(in.FilePath, in.Offset, in.Limit)
}

// readTextFile reads a file as text with optional offset/limit.
func (t *FileReadTool) readTextFile(filePath string, offsetPtr *int, limitPtr *int) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Sprintf("Error opening file: %v", err), nil
	}
	defer f.Close()

	// Determine offset and limit.
	offset := 1 // 1-based
	if offsetPtr != nil && *offsetPtr > 0 {
		offset = *offsetPtr
	}

	limit := fileReadDefaultLimit
	if limitPtr != nil && *limitPtr > 0 {
		limit = *limitPtr
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

// readImage reads an image file and returns base64-encoded content.
func (t *FileReadTool) readImage(filePath string, ext string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Sprintf("Error reading image: %v", err), nil
	}

	// Determine media type.
	var mediaType string
	switch ext {
	case ".png":
		mediaType = "image/png"
	case ".jpg", ".jpeg":
		mediaType = "image/jpeg"
	case ".gif":
		mediaType = "image/gif"
	case ".webp":
		mediaType = "image/webp"
	case ".bmp":
		mediaType = "image/bmp"
	default:
		mediaType = "application/octet-stream"
	}

	encoded := base64.StdEncoding.EncodeToString(data)

	result := map[string]interface{}{
		"type":       "image",
		"media_type": mediaType,
		"data":       encoded,
		"size":       len(data),
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// readPDF extracts text from a PDF file using pdftotext.
func (t *FileReadTool) readPDF(filePath string, pages string) (string, error) {
	args := []string{filePath, "-"}
	if pages != "" {
		// Parse page range for pdftotext -f (first) and -l (last) flags.
		parts := strings.SplitN(pages, "-", 2)
		if len(parts) == 2 {
			args = []string{"-f", parts[0], "-l", parts[1], filePath, "-"}
		} else if len(parts) == 1 {
			args = []string{"-f", parts[0], "-l", parts[0], filePath, "-"}
		}
	}

	cmd := exec.Command("pdftotext", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// pdftotext not available, try a basic info message.
		return fmt.Sprintf("Error: pdftotext not available to read PDF. Install poppler-utils. (%v)", err), nil
	}

	text := string(output)
	if text == "" {
		return "(empty PDF or no extractable text)", nil
	}

	// Truncate if very large.
	const maxPDFOutput = 200_000
	if len(text) > maxPDFOutput {
		text = text[:maxPDFOutput] + "\n... (PDF content truncated)"
	}

	return text, nil
}

// readNotebook reads a Jupyter notebook and renders all cells with outputs.
func (t *FileReadTool) readNotebook(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Sprintf("Error reading notebook: %v", err), nil
	}

	var notebook struct {
		Cells []struct {
			CellType string      `json:"cell_type"`
			Source   interface{} `json:"source"`
			Outputs  []struct {
				OutputType string      `json:"output_type"`
				Text       interface{} `json:"text"`
				Data       interface{} `json:"data"`
			} `json:"outputs"`
		} `json:"cells"`
	}

	if err := json.Unmarshal(data, &notebook); err != nil {
		return fmt.Sprintf("Error parsing notebook: %v", err), nil
	}

	var result strings.Builder
	for i, cell := range notebook.Cells {
		fmt.Fprintf(&result, "--- Cell %d [%s] ---\n", i+1, cell.CellType)

		// Render source.
		source := flattenNotebookSource(cell.Source)
		result.WriteString(source)
		if !strings.HasSuffix(source, "\n") {
			result.WriteString("\n")
		}

		// Render outputs.
		for _, out := range cell.Outputs {
			if out.Text != nil {
				text := flattenNotebookSource(out.Text)
				if text != "" {
					fmt.Fprintf(&result, "[Output]\n%s", text)
					if !strings.HasSuffix(text, "\n") {
						result.WriteString("\n")
					}
				}
			}
		}
		result.WriteString("\n")
	}

	output := result.String()
	if output == "" {
		return "(empty notebook)", nil
	}
	return output, nil
}

// flattenNotebookSource converts a notebook source field (string or []string) to a single string.
func flattenNotebookSource(source interface{}) string {
	switch v := source.(type) {
	case string:
		return v
	case []interface{}:
		var lines []string
		for _, line := range v {
			if s, ok := line.(string); ok {
				lines = append(lines, s)
			}
		}
		return strings.Join(lines, "")
	}
	return ""
}
