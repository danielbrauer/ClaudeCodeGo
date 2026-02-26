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

// AddServerToConfig adds an MCP server to the project-level .mcp.json file.
func AddServerToConfig(cwd, name, command string, args []string) error {
	path := filepath.Join(cwd, ".mcp.json")
	cfg, err := loadMCPFile(path)
	if err != nil {
		cfg = &MCPConfig{MCPServers: make(map[string]ServerConfig)}
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]ServerConfig)
	}

	cfg.MCPServers[name] = ServerConfig{
		Command: command,
		Args:    args,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// RemoveServerFromConfig removes an MCP server from the project-level .mcp.json file.
func RemoveServerFromConfig(cwd, name string) error {
	path := filepath.Join(cwd, ".mcp.json")
	cfg, err := loadMCPFile(path)
	if err != nil {
		return fmt.Errorf("no .mcp.json found")
	}

	if _, ok := cfg.MCPServers[name]; !ok {
		return fmt.Errorf("server %q not found in config", name)
	}

	delete(cfg.MCPServers, name)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
