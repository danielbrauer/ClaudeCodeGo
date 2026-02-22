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
}

// PermissionRule defines a tool permission rule.
type PermissionRule struct {
	Tool    string `json:"tool"`
	Pattern string `json:"pattern,omitempty"`
	Action  string `json:"action"` // "allow", "deny", "ask"
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
func loadSettingsFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
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

	return result
}
