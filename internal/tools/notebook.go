package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// NotebookEditInput is the input schema for the NotebookEdit tool.
type NotebookEditInput struct {
	NotebookPath string  `json:"notebook_path"`
	CellID       *string `json:"cell_id,omitempty"`
	NewSource    string  `json:"new_source"`
	CellType     *string `json:"cell_type,omitempty"`
	EditMode     *string `json:"edit_mode,omitempty"` // replace, insert, delete
}

// Notebook represents a Jupyter notebook file structure.
type Notebook struct {
	Cells         []NotebookCell         `json:"cells"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	NBFormat      int                    `json:"nbformat"`
	NBFormatMinor int                    `json:"nbformat_minor"`
}

// NotebookCell represents a single cell in a notebook.
type NotebookCell struct {
	ID             string                 `json:"id,omitempty"`
	CellType       string                 `json:"cell_type"`
	Source          interface{}            `json:"source"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Outputs        []interface{}          `json:"outputs,omitempty"`
	ExecutionCount *int                   `json:"execution_count,omitempty"`
}

// NotebookEditTool edits Jupyter notebook cells.
type NotebookEditTool struct{}

// NewNotebookEditTool creates a new NotebookEdit tool.
func NewNotebookEditTool() *NotebookEditTool {
	return &NotebookEditTool{}
}

func (t *NotebookEditTool) Name() string { return "NotebookEdit" }

func (t *NotebookEditTool) Description() string {
	return `Edit Jupyter notebook (.ipynb) cells. Supports replacing cell content, inserting new cells, and deleting cells. The notebook_path must be an absolute path. Use cell_id to target a specific cell. Use edit_mode to specify the operation (replace, insert, or delete).`
}

func (t *NotebookEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "notebook_path": {
      "type": "string",
      "description": "The absolute path to the Jupyter notebook file"
    },
    "cell_id": {
      "type": "string",
      "description": "The ID of the cell to edit. For insert mode, the new cell is inserted after this cell."
    },
    "new_source": {
      "type": "string",
      "description": "The new source for the cell"
    },
    "cell_type": {
      "type": "string",
      "enum": ["code", "markdown"],
      "description": "The type of the cell. Required for insert mode."
    },
    "edit_mode": {
      "type": "string",
      "enum": ["replace", "insert", "delete"],
      "description": "The type of edit (replace, insert, delete). Defaults to replace."
    }
  },
  "required": ["notebook_path", "new_source"],
  "additionalProperties": false
}`)
}

func (t *NotebookEditTool) RequiresPermission(_ json.RawMessage) bool {
	return true // writes to disk
}

func (t *NotebookEditTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in NotebookEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing NotebookEdit input: %w", err)
	}

	if in.NotebookPath == "" {
		return "Error: notebook_path is required", nil
	}

	if !filepath.IsAbs(in.NotebookPath) {
		return "Error: notebook_path must be an absolute path", nil
	}

	editMode := "replace"
	if in.EditMode != nil {
		editMode = *in.EditMode
	}

	// Read the notebook file.
	data, err := os.ReadFile(in.NotebookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: notebook not found: %s", in.NotebookPath), nil
		}
		return fmt.Sprintf("Error reading notebook: %v", err), nil
	}

	originalFile := string(data)

	var notebook Notebook
	if err := json.Unmarshal(data, &notebook); err != nil {
		return fmt.Sprintf("Error parsing notebook: %v", err), nil
	}

	var resultCellID string
	var resultCellType string

	switch editMode {
	case "replace":
		idx := t.findCellIndex(&notebook, in.CellID)
		if idx < 0 {
			return "Error: cell not found", nil
		}
		notebook.Cells[idx].Source = splitSource(in.NewSource)
		if in.CellType != nil {
			notebook.Cells[idx].CellType = *in.CellType
		}
		resultCellID = notebook.Cells[idx].ID
		resultCellType = notebook.Cells[idx].CellType

	case "insert":
		cellType := "code"
		if in.CellType != nil {
			cellType = *in.CellType
		}
		newCell := NotebookCell{
			ID:       generateCellID(),
			CellType: cellType,
			Source:   splitSource(in.NewSource),
			Metadata: map[string]interface{}{},
		}
		if cellType == "code" {
			newCell.Outputs = []interface{}{}
		}

		insertIdx := 0
		if in.CellID != nil {
			idx := t.findCellIndex(&notebook, in.CellID)
			if idx >= 0 {
				insertIdx = idx + 1
			}
		}

		// Insert cell at position.
		notebook.Cells = append(notebook.Cells, NotebookCell{})
		copy(notebook.Cells[insertIdx+1:], notebook.Cells[insertIdx:])
		notebook.Cells[insertIdx] = newCell

		resultCellID = newCell.ID
		resultCellType = newCell.CellType

	case "delete":
		idx := t.findCellIndex(&notebook, in.CellID)
		if idx < 0 {
			return "Error: cell not found", nil
		}
		resultCellID = notebook.Cells[idx].ID
		resultCellType = notebook.Cells[idx].CellType
		notebook.Cells = append(notebook.Cells[:idx], notebook.Cells[idx+1:]...)

	default:
		return fmt.Sprintf("Error: unknown edit_mode: %s", editMode), nil
	}

	// Write back the notebook.
	output, err := json.MarshalIndent(notebook, "", " ")
	if err != nil {
		return fmt.Sprintf("Error marshaling notebook: %v", err), nil
	}
	output = append(output, '\n')

	if err := os.WriteFile(in.NotebookPath, output, 0644); err != nil {
		return fmt.Sprintf("Error writing notebook: %v", err), nil
	}

	// Detect language from notebook metadata.
	language := "python"
	if notebook.Metadata != nil {
		if kernelspec, ok := notebook.Metadata["kernelspec"].(map[string]interface{}); ok {
			if lang, ok := kernelspec["language"].(string); ok {
				language = lang
			}
		}
	}

	result := map[string]interface{}{
		"new_source":    in.NewSource,
		"cell_id":       resultCellID,
		"cell_type":     resultCellType,
		"language":      language,
		"edit_mode":     editMode,
		"notebook_path": in.NotebookPath,
		"original_file": originalFile,
		"updated_file":  string(output),
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// findCellIndex finds a cell by ID or returns 0 if no ID is specified.
func (t *NotebookEditTool) findCellIndex(notebook *Notebook, cellID *string) int {
	if cellID == nil || *cellID == "" {
		if len(notebook.Cells) > 0 {
			return 0
		}
		return -1
	}

	for i, cell := range notebook.Cells {
		if cell.ID == *cellID {
			return i
		}
	}
	return -1
}

// splitSource converts a string into an array of lines for notebook source format.
func splitSource(source string) []string {
	if source == "" {
		return []string{}
	}
	lines := strings.Split(source, "\n")
	// Notebook source lines include the newline character, except the last line.
	result := make([]string, len(lines))
	for i, line := range lines {
		if i < len(lines)-1 {
			result[i] = line + "\n"
		} else {
			result[i] = line
		}
	}
	return result
}

// generateCellID creates a simple unique cell ID.
func generateCellID() string {
	return fmt.Sprintf("cell-%d", cellIDCounter.Add(1))
}

// cellIDCounter is an atomic counter for generating cell IDs.
var cellIDCounter atomicCounter

type atomicCounter struct {
	mu  sync.Mutex
	val int64
}

func (c *atomicCounter) Add(delta int64) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.val += delta
	return c.val
}
