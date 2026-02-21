package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GrepInput is the input schema for the Grep tool.
type GrepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`
	OutputMode string `json:"output_mode,omitempty"` // content | files_with_matches | count
	Before     *int   `json:"-B,omitempty"`
	After      *int   `json:"-A,omitempty"`
	CtxLines   *int   `json:"-C,omitempty"`
	Context    *int   `json:"context,omitempty"`
	LineNums   *bool  `json:"-n,omitempty"`
	IgnoreCase *bool  `json:"-i,omitempty"`
	FileType   string `json:"type,omitempty"`
	HeadLimit  *int   `json:"head_limit,omitempty"`
	Offset     *int   `json:"offset,omitempty"`
	Multiline  *bool  `json:"multiline,omitempty"`
}

// GrepTool searches file contents using ripgrep.
type GrepTool struct {
	workDir string
}

// NewGrepTool creates a new Grep tool with the given working directory.
func NewGrepTool(workDir string) *GrepTool {
	return &GrepTool{workDir: workDir}
}

func (t *GrepTool) Name() string { return "Grep" }

func (t *GrepTool) Description() string {
	return `Content search using regular expressions (ripgrep-compatible). Output modes: "content" shows matching lines with context, "files_with_matches" (default) shows only file paths, "count" shows match counts.`
}

func (t *GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "The regular expression pattern to search for in file contents"
    },
    "path": {
      "type": "string",
      "description": "File or directory to search in. Defaults to current working directory."
    },
    "glob": {
      "type": "string",
      "description": "Glob pattern to filter files (e.g. \"*.js\", \"*.{ts,tsx}\") - maps to rg --glob"
    },
    "output_mode": {
      "type": "string",
      "enum": ["content", "files_with_matches", "count"],
      "description": "Output mode. Defaults to \"files_with_matches\"."
    },
    "-B": {
      "type": "number",
      "description": "Number of lines to show before each match (rg -B). Requires output_mode: \"content\"."
    },
    "-A": {
      "type": "number",
      "description": "Number of lines to show after each match (rg -A). Requires output_mode: \"content\"."
    },
    "-C": {
      "type": "number",
      "description": "Alias for context. Lines before and after each match."
    },
    "context": {
      "type": "number",
      "description": "Number of lines to show before and after each match (rg -C). Requires output_mode: \"content\"."
    },
    "-n": {
      "type": "boolean",
      "description": "Show line numbers in output (rg -n). Requires output_mode: \"content\". Defaults to true."
    },
    "-i": {
      "type": "boolean",
      "description": "Case insensitive search (rg -i)"
    },
    "type": {
      "type": "string",
      "description": "File type to search (rg --type). Common types: js, py, rust, go, java, etc."
    },
    "head_limit": {
      "type": "number",
      "description": "Limit output to first N lines/entries. Defaults to 0 (unlimited)."
    },
    "offset": {
      "type": "number",
      "description": "Skip first N lines/entries before applying head_limit. Defaults to 0."
    },
    "multiline": {
      "type": "boolean",
      "description": "Enable multiline mode where . matches newlines (rg -U --multiline-dotall). Default: false."
    }
  },
  "required": ["pattern"],
  "additionalProperties": false
}`)
}

func (t *GrepTool) RequiresPermission(_ json.RawMessage) bool {
	return false // Read-only.
}

func (t *GrepTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	// We need custom unmarshaling because of the dash-prefixed keys.
	var in GrepInput
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(input, &raw); err != nil {
		return "", fmt.Errorf("parsing Grep input: %w", err)
	}

	// Parse standard fields.
	if v, ok := raw["pattern"]; ok {
		json.Unmarshal(v, &in.Pattern)
	}
	if v, ok := raw["path"]; ok {
		json.Unmarshal(v, &in.Path)
	}
	if v, ok := raw["glob"]; ok {
		json.Unmarshal(v, &in.Glob)
	}
	if v, ok := raw["output_mode"]; ok {
		json.Unmarshal(v, &in.OutputMode)
	}
	if v, ok := raw["type"]; ok {
		json.Unmarshal(v, &in.FileType)
	}
	if v, ok := raw["context"]; ok {
		var n int
		json.Unmarshal(v, &n)
		in.Context = &n
	}
	if v, ok := raw["head_limit"]; ok {
		var n int
		json.Unmarshal(v, &n)
		in.HeadLimit = &n
	}
	if v, ok := raw["offset"]; ok {
		var n int
		json.Unmarshal(v, &n)
		in.Offset = &n
	}

	// Parse dash-prefixed fields.
	if v, ok := raw["-B"]; ok {
		var n int
		json.Unmarshal(v, &n)
		in.Before = &n
	}
	if v, ok := raw["-A"]; ok {
		var n int
		json.Unmarshal(v, &n)
		in.After = &n
	}
	if v, ok := raw["-C"]; ok {
		var n int
		json.Unmarshal(v, &n)
		in.CtxLines = &n
	}
	if v, ok := raw["-n"]; ok {
		var b bool
		json.Unmarshal(v, &b)
		in.LineNums = &b
	}
	if v, ok := raw["-i"]; ok {
		var b bool
		json.Unmarshal(v, &b)
		in.IgnoreCase = &b
	}
	if v, ok := raw["multiline"]; ok {
		var b bool
		json.Unmarshal(v, &b)
		in.Multiline = &b
	}

	if in.Pattern == "" {
		return "Error: pattern is required", nil
	}

	// Check if rg is available.
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return t.fallbackGrep(ctx, &in)
	}

	return t.executeRipgrep(ctx, rgPath, &in)
}

func (t *GrepTool) executeRipgrep(ctx context.Context, rgPath string, in *GrepInput) (string, error) {
	args := []string{}

	// Output mode.
	mode := in.OutputMode
	if mode == "" {
		mode = "files_with_matches"
	}

	switch mode {
	case "files_with_matches":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count")
	case "content":
		// Default rg output mode.
		// Line numbers: default to true for content mode.
		showLineNums := true
		if in.LineNums != nil {
			showLineNums = *in.LineNums
		}
		if showLineNums {
			args = append(args, "-n")
		}
	}

	// Context lines (only for content mode).
	if mode == "content" {
		if in.Before != nil {
			args = append(args, "-B", fmt.Sprintf("%d", *in.Before))
		}
		if in.After != nil {
			args = append(args, "-A", fmt.Sprintf("%d", *in.After))
		}
		// -C and context are aliases.
		ctxLines := in.CtxLines
		if ctxLines == nil {
			ctxLines = in.Context
		}
		if ctxLines != nil {
			args = append(args, "-C", fmt.Sprintf("%d", *ctxLines))
		}
	}

	// Case insensitive.
	if in.IgnoreCase != nil && *in.IgnoreCase {
		args = append(args, "-i")
	}

	// File type.
	if in.FileType != "" {
		args = append(args, "--type", in.FileType)
	}

	// Glob filter.
	if in.Glob != "" {
		args = append(args, "--glob", in.Glob)
	}

	// Multiline.
	if in.Multiline != nil && *in.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}

	// Pattern.
	args = append(args, "--", in.Pattern)

	// Search path.
	searchPath := t.workDir
	if in.Path != "" {
		searchPath = in.Path
	}
	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, rgPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// rg exits with 1 when no matches found.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return "No matches found.", nil
			}
			if exitErr.ExitCode() == 2 && stderr.Len() > 0 {
				return fmt.Sprintf("Error: %s", strings.TrimSpace(stderr.String())), nil
			}
		}
		if ctx.Err() != nil {
			return "Search timed out.", nil
		}
		return fmt.Sprintf("Error running ripgrep: %v", err), nil
	}

	output := stdout.String()

	// Apply offset and head_limit.
	output = applyOffsetLimit(output, in.Offset, in.HeadLimit)

	if output == "" {
		return "No matches found.", nil
	}

	return strings.TrimRight(output, "\n"), nil
}

// fallbackGrep uses Go's built-in grep when ripgrep is not available.
func (t *GrepTool) fallbackGrep(ctx context.Context, in *GrepInput) (string, error) {
	// Fall back to standard grep.
	args := []string{"-r", "--include=*"}

	if in.IgnoreCase != nil && *in.IgnoreCase {
		args = append(args, "-i")
	}

	mode := in.OutputMode
	if mode == "" {
		mode = "files_with_matches"
	}

	switch mode {
	case "files_with_matches":
		args = append(args, "-l")
	case "count":
		args = append(args, "-c")
	case "content":
		args = append(args, "-n")
	}

	if in.Glob != "" {
		args = append(args, "--include="+in.Glob)
	}

	args = append(args, "--", in.Pattern)

	searchPath := t.workDir
	if in.Path != "" {
		searchPath = in.Path
	}
	args = append(args, searchPath)

	grepPath, err := exec.LookPath("grep")
	if err != nil {
		return "Error: neither ripgrep (rg) nor grep found on the system.", nil
	}

	cmd := exec.CommandContext(ctx, grepPath, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	cmd.Run() // ignore error, grep returns 1 for no matches

	output := stdout.String()
	output = applyOffsetLimit(output, in.Offset, in.HeadLimit)

	if output == "" {
		return "No matches found.", nil
	}

	return strings.TrimRight(output, "\n"), nil
}

// applyOffsetLimit applies line offset and limit to output text.
func applyOffsetLimit(output string, offset, headLimit *int) string {
	if (offset == nil || *offset == 0) && (headLimit == nil || *headLimit == 0) {
		return output
	}

	lines := strings.Split(output, "\n")

	off := 0
	if offset != nil && *offset > 0 {
		off = *offset
	}
	if off >= len(lines) {
		return ""
	}
	lines = lines[off:]

	if headLimit != nil && *headLimit > 0 && *headLimit < len(lines) {
		lines = lines[:*headLimit]
	}

	return strings.Join(lines, "\n")
}
