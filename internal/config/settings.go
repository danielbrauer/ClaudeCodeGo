// Package config handles settings loading, merging, and CLAUDE.md processing.
//
// Settings are loaded from five levels (highest priority first):
//  1. Managed — /etc/claude/settings.json
//  2. CLI flags — applied after loading (not handled here)
//  3. Local — .claude/settings.local.json (gitignored, per-project)
//  4. Project — .claude/settings.json (committed, per-project)
//  5. User — ~/.claude/settings.json (global)
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Settings holds merged configuration from all levels.
type Settings struct {
	Permissions []PermissionRule  `json:"permissions,omitempty"`
	Model       string            `json:"model,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Hooks       json.RawMessage   `json:"hooks,omitempty"` // parsed later in Phase 7
	Sandbox     json.RawMessage   `json:"sandbox,omitempty"`

	// User-facing preferences (displayed in the config panel).
	AutoCompactEnabled  *bool  `json:"autoCompactEnabled,omitempty"`
	Verbose             *bool  `json:"verbose,omitempty"`
	ThinkingEnabled     *bool  `json:"alwaysThinkingEnabled,omitempty"`
	EditorMode          string `json:"editorMode,omitempty"`   // "normal" or "vim"
	DiffTool            string `json:"diffTool,omitempty"`     // "terminal" or "auto"
	NotifChannel        string `json:"notifChannel,omitempty"` // "auto", "terminal_bell", "iterm2", etc.
	Theme               string `json:"theme,omitempty"`
	RespectGitignore    *bool  `json:"respectGitignore,omitempty"`
	FastMode            *bool  `json:"fastMode,omitempty"`
}

// PermissionRule defines a tool permission rule.
// Compatible with both the Go format ({tool, pattern, action}) and the JS format.
type PermissionRule struct {
	Tool    string `json:"tool"`
	Pattern string `json:"pattern,omitempty"`
	Action  string `json:"action"` // "allow", "deny", "ask"
}

// jsPermissions represents the JS-format permissions block:
//
//	{ "allow": ["Bash(npm:*)", "Read"], "deny": ["Bash(rm *)"], "ask": ["Write"] }
type jsPermissions struct {
	Allow                []string `json:"allow,omitempty"`
	Deny                 []string `json:"deny,omitempty"`
	Ask                  []string `json:"ask,omitempty"`
	DefaultMode          string   `json:"defaultMode,omitempty"`
	AdditionalDirectories []string `json:"additionalDirectories,omitempty"`
}

// rawSettings is used for initial JSON deserialization before normalizing
// the permissions format.
type rawSettings struct {
	Permissions json.RawMessage   `json:"permissions,omitempty"`
	Model       string            `json:"model,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Hooks       json.RawMessage   `json:"hooks,omitempty"`
	Sandbox     json.RawMessage   `json:"sandbox,omitempty"`

	// User-facing preferences.
	AutoCompactEnabled *bool  `json:"autoCompactEnabled,omitempty"`
	Verbose            *bool  `json:"verbose,omitempty"`
	ThinkingEnabled    *bool  `json:"alwaysThinkingEnabled,omitempty"`
	EditorMode         string `json:"editorMode,omitempty"`
	DiffTool           string `json:"diffTool,omitempty"`
	NotifChannel       string `json:"notifChannel,omitempty"`
	Theme              string `json:"theme,omitempty"`
	RespectGitignore   *bool  `json:"respectGitignore,omitempty"`
	FastMode           *bool  `json:"fastMode,omitempty"`
}

// LoadSettings loads and merges settings from all five levels.
// The merge order is user → project → local → managed (each level overrides the previous).
// Permission rules are concatenated, not replaced; higher-priority levels come first.
func LoadSettings(cwd string) (*Settings, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return &Settings{}, nil // non-fatal: use empty settings
	}

	paths := settingsPaths(home, cwd)

	// Load from lowest to highest priority, merging as we go.
	// Higher priority settings override lower priority ones.
	merged := &Settings{}
	for _, path := range paths {
		layer, err := loadSettingsFile(path)
		if err != nil {
			continue // file doesn't exist or is invalid — skip
		}
		merged = mergeSettings(merged, layer)
	}

	return merged, nil
}

// settingsPaths returns settings file paths from lowest to highest priority.
func settingsPaths(home, cwd string) []string {
	return []string{
		// 5. User (lowest priority)
		filepath.Join(home, ".claude", "settings.json"),
		// 4. Project
		filepath.Join(cwd, ".claude", "settings.json"),
		// 3. Local
		filepath.Join(cwd, ".claude", "settings.local.json"),
		// 1. Managed (highest priority)
		"/etc/claude/settings.json",
	}
}

// loadSettingsFile reads and parses a single settings JSON file.
// It supports both the Go format (flat rule array) and the JS format
// (permissions: {allow:[], deny:[], ask:[]}).
func loadSettingsFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw rawSettings
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	s := &Settings{
		Model:              raw.Model,
		Env:                raw.Env,
		Hooks:              raw.Hooks,
		Sandbox:            raw.Sandbox,
		AutoCompactEnabled: raw.AutoCompactEnabled,
		Verbose:            raw.Verbose,
		ThinkingEnabled:    raw.ThinkingEnabled,
		EditorMode:         raw.EditorMode,
		DiffTool:           raw.DiffTool,
		NotifChannel:       raw.NotifChannel,
		Theme:              raw.Theme,
		RespectGitignore:   raw.RespectGitignore,
		FastMode:           raw.FastMode,
	}

	// Parse permissions: try JS format first, then Go format.
	if raw.Permissions != nil {
		rules, err := parsePermissions(raw.Permissions)
		if err != nil {
			return nil, err
		}
		s.Permissions = rules
	}

	return s, nil
}

// parsePermissions handles both permission formats:
//   - JS format: {"allow": ["Bash(npm:*)"], "deny": [...], "ask": [...]}
//   - Go format: [{"tool": "Bash", "pattern": "npm:*", "action": "allow"}, ...]
func parsePermissions(data json.RawMessage) ([]PermissionRule, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// Determine the type by looking at the first non-whitespace byte.
	trimmed := trimJSONWhitespace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}

	switch trimmed[0] {
	case '{':
		// JS format: {allow: [...], deny: [...], ask: [...]}
		return parseJSPermissions(data)
	case '[':
		// Go format: [{tool, pattern, action}, ...]
		var rules []PermissionRule
		if err := json.Unmarshal(data, &rules); err != nil {
			return nil, err
		}
		return rules, nil
	default:
		return nil, nil
	}
}

// parseJSPermissions parses the JS-format permissions block into our internal
// PermissionRule slice. Rule strings like "Bash(npm:*)" are parsed into
// {Tool: "Bash", Pattern: "npm:*", Action: "allow"}.
func parseJSPermissions(data json.RawMessage) ([]PermissionRule, error) {
	var jp jsPermissions
	if err := json.Unmarshal(data, &jp); err != nil {
		return nil, err
	}

	var rules []PermissionRule

	for _, s := range jp.Allow {
		rule := ParseRuleString(s)
		rule.Action = "allow"
		rules = append(rules, rule)
	}
	for _, s := range jp.Deny {
		rule := ParseRuleString(s)
		rule.Action = "deny"
		rules = append(rules, rule)
	}
	for _, s := range jp.Ask {
		rule := ParseRuleString(s)
		rule.Action = "ask"
		rules = append(rules, rule)
	}

	return rules, nil
}

// ParseRuleString parses a rule string like "Bash(npm:*)" or "Read" into a
// PermissionRule. This matches the JS TW() function behavior.
//
// Format: ToolName or ToolName(pattern)
// Examples:
//
//	"Bash"           → {Tool: "Bash"}
//	"Bash(npm:*)"    → {Tool: "Bash", Pattern: "npm:*"}
//	"Read(src/**)"   → {Tool: "Read", Pattern: "src/**"}
//	"WebFetch(domain:example.com)" → {Tool: "WebFetch", Pattern: "domain:example.com"}
func ParseRuleString(s string) PermissionRule {
	// Find the first unescaped '('.
	parenIdx := findUnescaped(s, '(')
	if parenIdx == -1 {
		return PermissionRule{Tool: s}
	}

	// Find the last unescaped ')'.
	closeIdx := findLastUnescaped(s, ')')
	if closeIdx == -1 || closeIdx <= parenIdx || closeIdx != len(s)-1 {
		return PermissionRule{Tool: s}
	}

	toolName := s[:parenIdx]
	if toolName == "" {
		return PermissionRule{Tool: s}
	}

	content := s[parenIdx+1 : closeIdx]
	// Empty parens or just "*" means match all — same as no pattern.
	if content == "" || content == "*" {
		return PermissionRule{Tool: toolName}
	}

	// Unescape the content.
	content = unescapeRuleContent(content)

	return PermissionRule{Tool: toolName, Pattern: content}
}

// FormatRuleString converts a PermissionRule back to the JS string format.
// This is the inverse of ParseRuleString.
func FormatRuleString(r PermissionRule) string {
	if r.Pattern == "" {
		return r.Tool
	}
	return r.Tool + "(" + escapeRuleContent(r.Pattern) + ")"
}

// findUnescaped finds the first unescaped occurrence of ch in s.
func findUnescaped(s string, ch byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ch {
			backslashes := 0
			j := i - 1
			for j >= 0 && s[j] == '\\' {
				backslashes++
				j--
			}
			if backslashes%2 == 0 {
				return i
			}
		}
	}
	return -1
}

// findLastUnescaped finds the last unescaped occurrence of ch in s.
func findLastUnescaped(s string, ch byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ch {
			backslashes := 0
			j := i - 1
			for j >= 0 && s[j] == '\\' {
				backslashes++
				j--
			}
			if backslashes%2 == 0 {
				return i
			}
		}
	}
	return -1
}

// unescapeRuleContent removes escape sequences from rule content.
func unescapeRuleContent(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			if next == '(' || next == ')' || next == '\\' {
				result = append(result, next)
				i++
				continue
			}
		}
		result = append(result, s[i])
	}
	return string(result)
}

// escapeRuleContent escapes special characters in rule content for serialization.
func escapeRuleContent(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '(' || s[i] == ')' || s[i] == '\\' {
			result = append(result, '\\')
		}
		result = append(result, s[i])
	}
	return string(result)
}

// trimJSONWhitespace removes leading whitespace from JSON data.
func trimJSONWhitespace(data []byte) []byte {
	for i, b := range data {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return data[i:]
		}
	}
	return nil
}

// mergeSettings merges overlay on top of base.
// Scalar fields from overlay replace base when non-zero.
// Permission rules are concatenated with overlay rules first (higher priority).
// Env maps are merged with overlay values overriding base.
func mergeSettings(base, overlay *Settings) *Settings {
	result := &Settings{}

	// Model: overlay wins if set.
	result.Model = base.Model
	if overlay.Model != "" {
		result.Model = overlay.Model
	}

	// Permissions: concatenate (overlay first = higher priority).
	result.Permissions = append(result.Permissions, overlay.Permissions...)
	result.Permissions = append(result.Permissions, base.Permissions...)

	// Env: deep merge, overlay wins per key.
	result.Env = make(map[string]string)
	for k, v := range base.Env {
		result.Env[k] = v
	}
	for k, v := range overlay.Env {
		result.Env[k] = v
	}

	// Hooks: overlay wins if set.
	result.Hooks = base.Hooks
	if overlay.Hooks != nil {
		result.Hooks = overlay.Hooks
	}

	// Sandbox: overlay wins if set.
	result.Sandbox = base.Sandbox
	if overlay.Sandbox != nil {
		result.Sandbox = overlay.Sandbox
	}

	// User-facing preferences: overlay wins if set.
	result.AutoCompactEnabled = base.AutoCompactEnabled
	if overlay.AutoCompactEnabled != nil {
		result.AutoCompactEnabled = overlay.AutoCompactEnabled
	}
	result.Verbose = base.Verbose
	if overlay.Verbose != nil {
		result.Verbose = overlay.Verbose
	}
	result.ThinkingEnabled = base.ThinkingEnabled
	if overlay.ThinkingEnabled != nil {
		result.ThinkingEnabled = overlay.ThinkingEnabled
	}
	result.EditorMode = base.EditorMode
	if overlay.EditorMode != "" {
		result.EditorMode = overlay.EditorMode
	}
	result.DiffTool = base.DiffTool
	if overlay.DiffTool != "" {
		result.DiffTool = overlay.DiffTool
	}
	result.NotifChannel = base.NotifChannel
	if overlay.NotifChannel != "" {
		result.NotifChannel = overlay.NotifChannel
	}
	result.Theme = base.Theme
	if overlay.Theme != "" {
		result.Theme = overlay.Theme
	}
	result.RespectGitignore = base.RespectGitignore
	if overlay.RespectGitignore != nil {
		result.RespectGitignore = overlay.RespectGitignore
	}
	result.FastMode = base.FastMode
	if overlay.FastMode != nil {
		result.FastMode = overlay.FastMode
	}

	return result
}

// UserSettingsPath returns the path to the user-level settings file (~/.claude/settings.json).
func UserSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// SaveUserSetting saves a single key/value pair to the user-level settings file.
// It reads the existing file, deep-merges the new value, and writes back.
func SaveUserSetting(key string, value interface{}) error {
	path, err := UserSettingsPath()
	if err != nil {
		return err
	}

	// Read existing settings as raw map.
	var settings map[string]interface{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			settings = make(map[string]interface{})
			if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr != nil {
				return fmt.Errorf("creating settings directory: %w", mkErr)
			}
		} else {
			return fmt.Errorf("reading settings: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			// If the file is corrupt, start fresh rather than fail.
			settings = make(map[string]interface{})
		}
	}

	// nil means "remove the key" (matches JS CLI behavior of saving undefined).
	if value == nil {
		delete(settings, key)
	} else {
		settings[key] = value
	}

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	output = append(output, '\n')

	if err := os.WriteFile(path, output, 0644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	return nil
}

// BoolVal returns the value of a *bool pointer, or the default if nil.
func BoolVal(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// BoolPtr returns a pointer to a bool value.
func BoolPtr(v bool) *bool {
	return &v
}
