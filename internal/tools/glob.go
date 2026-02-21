package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// GlobInput is the input schema for the Glob tool.
type GlobInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// GlobTool performs file pattern matching.
type GlobTool struct {
	workDir string
}

// NewGlobTool creates a new Glob tool with the given working directory.
func NewGlobTool(workDir string) *GlobTool {
	return &GlobTool{workDir: workDir}
}

func (t *GlobTool) Name() string { return "Glob" }

func (t *GlobTool) Description() string {
	return `Fast file pattern matching tool. Supports glob patterns like "**/*.js" or "src/**/*.ts". Returns matching file paths sorted by modification time.`
}

func (t *GlobTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "The glob pattern to match files against"
    },
    "path": {
      "type": "string",
      "description": "The directory to search in. Defaults to the working directory if omitted."
    }
  },
  "required": ["pattern"],
  "additionalProperties": false
}`)
}

func (t *GlobTool) RequiresPermission(_ json.RawMessage) bool {
	return false // Read-only.
}

func (t *GlobTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in GlobInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing Glob input: %w", err)
	}

	if in.Pattern == "" {
		return "Error: pattern is required", nil
	}

	// Determine search directory.
	searchDir := t.workDir
	if in.Path != "" {
		if filepath.IsAbs(in.Path) {
			searchDir = in.Path
		} else {
			searchDir = filepath.Join(t.workDir, in.Path)
		}
	}

	// Verify directory exists.
	info, err := os.Stat(searchDir)
	if err != nil {
		return fmt.Sprintf("Error: directory not found: %s", searchDir), nil
	}
	if !info.IsDir() {
		return fmt.Sprintf("Error: %s is not a directory", searchDir), nil
	}

	// Use doublestar for ** support.
	fsys := os.DirFS(searchDir)
	matches, err := doublestar.Glob(fsys, in.Pattern)
	if err != nil {
		return fmt.Sprintf("Error matching pattern: %v", err), nil
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No files matched pattern: %s in %s", in.Pattern, searchDir), nil
	}

	// Convert to absolute paths and collect with mod times.
	type fileEntry struct {
		path    string
		modTime int64
	}
	var entries []fileEntry

	for _, m := range matches {
		absPath := filepath.Join(searchDir, m)
		fi, err := os.Stat(absPath)
		if err != nil {
			continue // skip files we can't stat
		}
		if fi.IsDir() {
			continue // skip directories, only return files
		}
		entries = append(entries, fileEntry{
			path:    absPath,
			modTime: fi.ModTime().UnixNano(),
		})
	}

	// Sort by modification time, most recent first.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime > entries[j].modTime
	})

	var result strings.Builder
	for _, e := range entries {
		result.WriteString(e.path)
		result.WriteString("\n")
	}

	return strings.TrimRight(result.String(), "\n"), nil
}
