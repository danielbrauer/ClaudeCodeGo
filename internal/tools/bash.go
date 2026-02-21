package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	bashDefaultTimeout = 120 * time.Second
	bashMaxTimeout     = 600 * time.Second
)

// BashInput is the input schema for the Bash tool.
type BashInput struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	Timeout     *int   `json:"timeout,omitempty"` // milliseconds
}

// BashTool executes shell commands.
type BashTool struct {
	workDir string
}

// NewBashTool creates a Bash tool that runs commands in the given directory.
func NewBashTool(workDir string) *BashTool {
	return &BashTool{workDir: workDir}
}

func (t *BashTool) Name() string { return "Bash" }

func (t *BashTool) Description() string {
	return `Executes a bash command. Use for running shell commands, scripts, installing packages, compiling code, managing files via CLI, or any other terminal task. Commands run in the working directory. Specify an optional timeout in milliseconds (max 600000ms / 10 minutes). Commands timeout after 120000ms (2 minutes) by default.`
}

func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "description": "The command to execute"
    },
    "description": {
      "type": "string",
      "description": "Clear, concise description of what this command does"
    },
    "timeout": {
      "type": "number",
      "description": "Optional timeout in milliseconds (max 600000)"
    }
  },
  "required": ["command"],
  "additionalProperties": false
}`)
}

func (t *BashTool) RequiresPermission(_ json.RawMessage) bool {
	return true
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in BashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing Bash input: %w", err)
	}

	if in.Command == "" {
		return "Error: command is required", nil
	}

	// Determine timeout.
	timeout := bashDefaultTimeout
	if in.Timeout != nil {
		d := time.Duration(*in.Timeout) * time.Millisecond
		if d > bashMaxTimeout {
			d = bashMaxTimeout
		}
		if d > 0 {
			timeout = d
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", in.Command)
	cmd.Dir = t.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		result.Write(stdout.Bytes())
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(stderr.String())
	}

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString("Command timed out")
			return result.String(), nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString(fmt.Sprintf("Exit code: %d", exitErr.ExitCode()))
			return result.String(), nil
		}

		return "", fmt.Errorf("executing command: %w", err)
	}

	output := result.String()
	if output == "" {
		output = "(no output)"
	}

	// Truncate very large outputs.
	const maxOutput = 100_000
	if len(output) > maxOutput {
		output = output[:maxOutput] + "\n... (output truncated)"
	}

	return output, nil
}
