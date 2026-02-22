package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ExitPlanModeInput is the input schema for the ExitPlanMode tool.
type ExitPlanModeInput struct {
	AllowedPrompts []AllowedPrompt `json:"allowedPrompts,omitempty"`
}

// AllowedPrompt describes a prompt-based permission for plan implementation.
type AllowedPrompt struct {
	Tool   string `json:"tool"`
	Prompt string `json:"prompt"`
}

// ExitPlanModeTool signals that the model has finished writing a plan.
type ExitPlanModeTool struct{}

// NewExitPlanModeTool creates a new ExitPlanMode tool.
func NewExitPlanModeTool() *ExitPlanModeTool {
	return &ExitPlanModeTool{}
}

func (t *ExitPlanModeTool) Name() string { return "ExitPlanMode" }

func (t *ExitPlanModeTool) Description() string {
	return `Signal that you have finished writing a plan and are ready for user approval. Use this when you have completed planning the implementation steps of a task.`
}

func (t *ExitPlanModeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "allowedPrompts": {
      "type": "array",
      "description": "Prompt-based permissions needed to implement the plan",
      "items": {
        "type": "object",
        "properties": {
          "tool": {
            "type": "string",
            "enum": ["Bash"],
            "description": "The tool this prompt applies to"
          },
          "prompt": {
            "type": "string",
            "description": "Semantic description of the action"
          }
        },
        "required": ["tool", "prompt"],
        "additionalProperties": false
      }
    }
  },
  "additionalProperties": false
}`)
}

func (t *ExitPlanModeTool) RequiresPermission(_ json.RawMessage) bool {
	return false
}

func (t *ExitPlanModeTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in ExitPlanModeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing ExitPlanMode input: %w", err)
	}

	msg := "Plan is ready for review."
	if len(in.AllowedPrompts) > 0 {
		msg += " The following actions will be needed:"
		for _, ap := range in.AllowedPrompts {
			msg += fmt.Sprintf("\n  - %s: %s", ap.Tool, ap.Prompt)
		}
	}

	result := map[string]interface{}{
		"status":  "plan_ready",
		"message": msg,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
