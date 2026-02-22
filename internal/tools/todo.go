package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// TodoItem represents a single task in the todo list.
type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm"`
}

// TodoWriteInput is the input schema for the TodoWrite tool.
type TodoWriteInput struct {
	Todos []TodoItem `json:"todos"`
}

// TodoWriteTool manages a structured task list.
type TodoWriteTool struct {
	mu    sync.Mutex
	todos []TodoItem
}

// NewTodoWriteTool creates a new TodoWrite tool.
func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
}

func (t *TodoWriteTool) Name() string { return "TodoWrite" }

func (t *TodoWriteTool) Description() string {
	return `Use this tool to create and manage a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user. Each todo item has content (imperative form), status (pending/in_progress/completed), and activeForm (present continuous form).`
}

func (t *TodoWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "description": "The updated todo list",
      "items": {
        "type": "object",
        "properties": {
          "content": {
            "type": "string",
            "minLength": 1
          },
          "status": {
            "type": "string",
            "enum": ["pending", "in_progress", "completed"]
          },
          "activeForm": {
            "type": "string",
            "minLength": 1
          }
        },
        "required": ["content", "status", "activeForm"],
        "additionalProperties": false
      }
    }
  },
  "required": ["todos"],
  "additionalProperties": false
}`)
}

func (t *TodoWriteTool) RequiresPermission(_ json.RawMessage) bool {
	return false
}

func (t *TodoWriteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in TodoWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing TodoWrite input: %w", err)
	}

	t.mu.Lock()
	oldTodos := make([]TodoItem, len(t.todos))
	copy(oldTodos, t.todos)
	t.todos = make([]TodoItem, len(in.Todos))
	copy(t.todos, in.Todos)
	t.mu.Unlock()

	// Print formatted task list to terminal.
	fmt.Println()
	for _, item := range in.Todos {
		var icon string
		switch item.Status {
		case "pending":
			icon = "[ ]"
		case "in_progress":
			icon = "[~]"
		case "completed":
			icon = "[x]"
		default:
			icon = "[ ]"
		}
		fmt.Printf("  %s %s\n", icon, item.Content)
	}
	fmt.Println()

	result := map[string]interface{}{
		"oldTodos": oldTodos,
		"newTodos": in.Todos,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
