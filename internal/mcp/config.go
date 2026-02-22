package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadMCPConfig loads and merges .mcp.json from the user home and project dirs.
// User-level config (~/.mcp.json) is loaded first; project-level (.mcp.json in
// cwd) overrides per server name.
func LoadMCPConfig(cwd string) (*MCPConfig, error) {
	merged := &MCPConfig{
		MCPServers: make(map[string]ServerConfig),
	}

	// 1. User-level: ~/.mcp.json
	home, err := os.UserHomeDir()
	if err == nil {
		userConfig := filepath.Join(home, ".mcp.json")
		if cfg, err := loadMCPFile(userConfig); err == nil {
			for name, sc := range cfg.MCPServers {
				merged.MCPServers[name] = sc
			}
		}
	}

	// 2. Project-level: <cwd>/.mcp.json (overrides user-level per server name)
	projectConfig := filepath.Join(cwd, ".mcp.json")
	if cfg, err := loadMCPFile(projectConfig); err == nil {
		for name, sc := range cfg.MCPServers {
			merged.MCPServers[name] = sc
		}
	}

	if len(merged.MCPServers) == 0 {
		return nil, nil
	}

	return merged, nil
}

// loadMCPFile reads and parses a single .mcp.json file.
func loadMCPFile(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err // file not found is normal
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return &cfg, nil
}
