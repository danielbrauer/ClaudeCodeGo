package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigInput is the input schema for the Config tool.
type ConfigInput struct {
	Setting string      `json:"setting"`
	Value   interface{} `json:"value,omitempty"`
}

// ConfigTool gets or sets configuration values at runtime.
type ConfigTool struct {
	cwd string
}

// NewConfigTool creates a new Config tool.
func NewConfigTool(cwd string) *ConfigTool {
	return &ConfigTool{cwd: cwd}
}

func (t *ConfigTool) Name() string { return "Config" }

func (t *ConfigTool) Description() string {
	return `Get or set configuration values. Omit value to get the current setting. Provide value to set it. Operates on ~/.claude/settings.json by default.`
}

func (t *ConfigTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "setting": {
      "type": "string",
      "description": "The setting path (e.g. \"model\", \"theme\")"
    },
    "value": {
      "description": "The value to set (omit to get current value)"
    }
  },
  "required": ["setting"],
  "additionalProperties": false
}`)
}

func (t *ConfigTool) RequiresPermission(_ json.RawMessage) bool {
	return false
}

func (t *ConfigTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in ConfigInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing Config input: %w", err)
	}

	if in.Setting == "" {
		return "Error: setting is required", nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Sprintf("Error: cannot determine home directory: %v", err), nil
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")

	if in.Value == nil {
		// Get operation.
		return t.getSetting(settingsPath, in.Setting)
	}

	// Set operation.
	return t.setSetting(settingsPath, in.Setting, in.Value)
}

func (t *ConfigTool) getSetting(path, setting string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			result := map[string]interface{}{
				"success":   true,
				"operation": "get",
				"setting":   setting,
				"value":     nil,
			}
			out, _ := json.Marshal(result)
			return string(out), nil
		}
		return fmt.Sprintf("Error reading settings: %v", err), nil
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Sprintf("Error parsing settings: %v", err), nil
	}

	value := getNestedValue(settings, setting)

	result := map[string]interface{}{
		"success":   true,
		"operation": "get",
		"setting":   setting,
		"value":     value,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

func (t *ConfigTool) setSetting(path, setting string, value interface{}) (string, error) {
	// Read existing settings.
	var settings map[string]interface{}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			settings = make(map[string]interface{})
			// Ensure directory exists.
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return fmt.Sprintf("Error creating settings directory: %v", err), nil
			}
		} else {
			return fmt.Sprintf("Error reading settings: %v", err), nil
		}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Sprintf("Error parsing settings: %v", err), nil
		}
	}

	previousValue := getNestedValue(settings, setting)
	setNestedValue(settings, setting, value)

	// Write back.
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling settings: %v", err), nil
	}
	output = append(output, '\n')

	if err := os.WriteFile(path, output, 0644); err != nil {
		return fmt.Sprintf("Error writing settings: %v", err), nil
	}

	result := map[string]interface{}{
		"success":       true,
		"operation":     "set",
		"setting":       setting,
		"previousValue": previousValue,
		"newValue":      value,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// getNestedValue retrieves a value from a nested map using dot-separated keys.
func getNestedValue(m map[string]interface{}, path string) interface{} {
	keys := strings.Split(path, ".")
	current := interface{}(m)

	for _, key := range keys {
		obj, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current, ok = obj[key]
		if !ok {
			return nil
		}
	}
	return current
}

// setNestedValue sets a value in a nested map using dot-separated keys.
func setNestedValue(m map[string]interface{}, path string, value interface{}) {
	keys := strings.Split(path, ".")
	current := m

	for i, key := range keys {
		if i == len(keys)-1 {
			current[key] = value
			return
		}
		next, ok := current[key].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[key] = next
		}
		current = next
	}
}
