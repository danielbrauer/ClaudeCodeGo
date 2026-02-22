package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TaskOutputInput is the input schema for the TaskOutput tool.
type TaskOutputInput struct {
	TaskID  string `json:"task_id"`
	Block   bool   `json:"block,omitempty"`
	Timeout *int   `json:"timeout,omitempty"` // milliseconds
}

// TaskOutputTool reads the output of a background agent or command.
type TaskOutputTool struct {
	bgStore *BackgroundTaskStore
}

// NewTaskOutputTool creates a new TaskOutput tool.
func NewTaskOutputTool(bgStore *BackgroundTaskStore) *TaskOutputTool {
	return &TaskOutputTool{bgStore: bgStore}
}

func (t *TaskOutputTool) Name() string { return "TaskOutput" }

func (t *TaskOutputTool) Description() string {
	return `Read output from a background task. Use task_id to identify the task. Set block=true to wait for completion. Use timeout (ms) to limit how long to wait.`
}

func (t *TaskOutputTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "The ID of the background task"
    },
    "block": {
      "type": "boolean",
      "description": "Whether to wait for completion"
    },
    "timeout": {
      "type": "number",
      "description": "Max wait time in ms"
    }
  },
  "required": ["task_id"],
  "additionalProperties": false
}`)
}

func (t *TaskOutputTool) RequiresPermission(_ json.RawMessage) bool {
	return false
}

func (t *TaskOutputTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in TaskOutputInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing TaskOutput input: %w", err)
	}

	if in.TaskID == "" {
		return "Error: task_id is required", nil
	}

	task, ok := t.bgStore.Get(in.TaskID)
	if !ok {
		return fmt.Sprintf("Error: task %s not found", in.TaskID), nil
	}

	if in.Block {
		timeout := 60 * time.Second // default 60s
		if in.Timeout != nil && *in.Timeout > 0 {
			timeout = time.Duration(*in.Timeout) * time.Millisecond
		}

		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-task.Done:
			// Task completed.
		case <-timer.C:
			result := map[string]interface{}{
				"status":  "timeout",
				"taskId":  in.TaskID,
				"message": "Task did not complete within timeout",
			}
			out, _ := json.Marshal(result)
			return string(out), nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	// Check if task is done.
	select {
	case <-task.Done:
		status := "completed"
		if task.Err != nil {
			status = "error"
		}
		result := map[string]interface{}{
			"status": status,
			"taskId": in.TaskID,
			"output": task.Result,
		}
		if task.Err != nil {
			result["error"] = task.Err.Error()
		}
		out, _ := json.Marshal(result)
		return string(out), nil
	default:
		result := map[string]interface{}{
			"status":  "running",
			"taskId":  in.TaskID,
			"message": "Task is still running",
		}
		out, _ := json.Marshal(result)
		return string(out), nil
	}
}
