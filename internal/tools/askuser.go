package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// AskUserOption represents a single choice in a question.
type AskUserOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AskUserQuestionItem represents a single question.
type AskUserQuestionItem struct {
	Question    string          `json:"question"`
	Header      string          `json:"header"`
	Options     []AskUserOption `json:"options"`
	MultiSelect bool            `json:"multiSelect"`
}

// AskUserInput is the input schema for the AskUserQuestion tool.
type AskUserInput struct {
	Questions []AskUserQuestionItem `json:"questions"`
}

// AskUserRequestMsg is sent to the BT program when user input is needed.
// This type is defined here to avoid import cycles (tui imports tools).
type AskUserRequestMsg struct {
	Questions  []AskUserQuestionItem
	ResponseCh chan map[string]string
}

// AskUserTool presents structured questions with options to the user.
type AskUserTool struct {
	reader  *bufio.Reader
	program *tea.Program // nil in print mode
}

// NewAskUserTool creates a new AskUserQuestion tool (print mode fallback).
func NewAskUserTool() *AskUserTool {
	return &AskUserTool{
		reader: bufio.NewReader(os.Stdin),
	}
}

// NewAskUserToolWithProgram creates an AskUser tool wired to a BT program.
func NewAskUserToolWithProgram(p *tea.Program) *AskUserTool {
	return &AskUserTool{
		reader:  bufio.NewReader(os.Stdin),
		program: p,
	}
}

// SetProgram sets the BT program after construction (for late wiring).
func (t *AskUserTool) SetProgram(p *tea.Program) {
	t.program = p
}

func (t *AskUserTool) Name() string { return "AskUserQuestion" }

func (t *AskUserTool) Description() string {
	return `Use this tool when you need to ask the user questions during execution. This allows you to gather user preferences or requirements, clarify ambiguous instructions, get decisions on implementation choices, and offer choices to the user about what direction to take.`
}

func (t *AskUserTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "questions": {
      "type": "array",
      "description": "Questions to ask the user (1-4 questions)",
      "items": {
        "type": "object",
        "properties": {
          "question": {
            "type": "string",
            "description": "The complete question to ask the user"
          },
          "header": {
            "type": "string",
            "description": "Very short label (max 12 chars)"
          },
          "options": {
            "type": "array",
            "description": "The available choices (2-4 options)",
            "items": {
              "type": "object",
              "properties": {
                "label": {
                  "type": "string",
                  "description": "Display text for the option"
                },
                "description": {
                  "type": "string",
                  "description": "Explanation of what this option means"
                }
              },
              "required": ["label", "description"],
              "additionalProperties": false
            },
            "minItems": 2,
            "maxItems": 4
          },
          "multiSelect": {
            "type": "boolean",
            "default": false,
            "description": "Allow multiple selections"
          }
        },
        "required": ["question", "header", "options", "multiSelect"],
        "additionalProperties": false
      },
      "minItems": 1,
      "maxItems": 4
    }
  },
  "required": ["questions"],
  "additionalProperties": false
}`)
}

func (t *AskUserTool) RequiresPermission(_ json.RawMessage) bool {
	return false // user interaction IS the permission
}

func (t *AskUserTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in AskUserInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing AskUserQuestion input: %w", err)
	}

	if len(in.Questions) == 0 {
		return "Error: at least one question is required", nil
	}

	// TUI mode: delegate to the BT event loop via channel handshake.
	if t.program != nil {
		responseCh := make(chan map[string]string, 1)
		t.program.Send(AskUserRequestMsg{
			Questions:  in.Questions,
			ResponseCh: responseCh,
		})

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case answers := <-responseCh:
			result := map[string]interface{}{
				"questions": in.Questions,
				"answers":   answers,
			}
			out, _ := json.Marshal(result)
			return string(out), nil
		}
	}

	// Print mode fallback: use terminal stdin.
	return t.executeTerminal(ctx, in)
}

// executeTerminal handles AskUser in non-TUI mode using fmt/bufio.
func (t *AskUserTool) executeTerminal(ctx context.Context, in AskUserInput) (string, error) {
	answers := make(map[string]string)

	for _, q := range in.Questions {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		fmt.Printf("\n[%s] %s\n", q.Header, q.Question)
		for i, opt := range q.Options {
			fmt.Printf("  %d. %s - %s\n", i+1, opt.Label, opt.Description)
		}
		otherIdx := len(q.Options) + 1
		fmt.Printf("  %d. Other (provide custom input)\n", otherIdx)

		if q.MultiSelect {
			fmt.Print("Select options (comma-separated numbers): ")
		} else {
			fmt.Print("Select an option: ")
		}

		line, err := t.reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading user input: %w", err)
		}

		line = strings.TrimSpace(line)

		if q.MultiSelect {
			var selected []string
			parts := strings.Split(line, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				idx, err := strconv.Atoi(p)
				if err != nil || idx < 1 || idx > otherIdx {
					continue
				}
				if idx == otherIdx {
					fmt.Print("Enter your custom input: ")
					custom, err := t.reader.ReadString('\n')
					if err != nil {
						return "", fmt.Errorf("reading custom input: %w", err)
					}
					selected = append(selected, strings.TrimSpace(custom))
				} else {
					selected = append(selected, q.Options[idx-1].Label)
				}
			}
			answers[q.Question] = strings.Join(selected, ", ")
		} else {
			idx, err := strconv.Atoi(line)
			if err != nil || idx < 1 || idx > otherIdx {
				// Treat as free text input.
				answers[q.Question] = line
			} else if idx == otherIdx {
				fmt.Print("Enter your custom input: ")
				custom, readErr := t.reader.ReadString('\n')
				if readErr != nil {
					return "", fmt.Errorf("reading custom input: %w", readErr)
				}
				answers[q.Question] = strings.TrimSpace(custom)
			} else {
				answers[q.Question] = q.Options[idx-1].Label
			}
		}
	}

	result := map[string]interface{}{
		"questions": in.Questions,
		"answers":   answers,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
