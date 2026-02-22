package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TaskStopInput is the input schema for the TaskStop tool.
type TaskStopInput struct {
	TaskID  *string `json:"task_id,omitempty"`
	ShellID *string `json:"shell_id,omitempty"` // deprecated alias for task_id
}

// TaskStopTool stops a running background task.
type TaskStopTool struct {
	bgStore *BackgroundTaskStore
}

// NewTaskStopTool creates a new TaskStop tool.
func NewTaskStopTool(bgStore *BackgroundTaskStore) *TaskStopTool {
	return &TaskStopTool{bgStore: bgStore}
}

func (t *TaskStopTool) Name() string { return "TaskStop" }

func (t *TaskStopTool) Description() string {
	return `Stop a running background task by its ID. Use task_id to identify the task to stop.`
}

func (t *TaskStopTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "The ID of the background task to stop"
    },
    "shell_id": {
      "type": "string",
      "description": "Deprecated alias for task_id"
    }
  },
  "additionalProperties": false
}`)
}

func (t *TaskStopTool) RequiresPermission(_ json.RawMessage) bool {
	return false
}

func (t *TaskStopTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in TaskStopInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing TaskStop input: %w", err)
	}

	// Resolve task ID (task_id takes priority over shell_id).
	taskID := ""
	if in.TaskID != nil {
		taskID = *in.TaskID
	} else if in.ShellID != nil {
		taskID = *in.ShellID
	}

	if taskID == "" {
		return "Error: task_id or shell_id is required", nil
	}

	task, ok := t.bgStore.Get(taskID)
	if !ok {
		return fmt.Sprintf("Error: task %s not found", taskID), nil
	}

	// Cancel the task's context.
	if task.Cancel != nil {
		task.Cancel()
	}

	// Wait briefly for cleanup.
	select {
	case <-task.Done:
	case <-time.After(5 * time.Second):
	}

	result := map[string]interface{}{
		"status":  "stopped",
		"taskId":  taskID,
		"message": fmt.Sprintf("Task %s has been stopped", taskID),
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
