package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMCPConfig_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := LoadMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config when no .mcp.json files exist")
	}
}

func TestLoadMCPConfig_ProjectLevel(t *testing.T) {
	tmpDir := t.TempDir()

	mcpJSON := `{
		"mcpServers": {
			"test-server": {
				"command": "echo",
				"args": ["hello"]
			}
		}
	}`

	err := os.WriteFile(filepath.Join(tmpDir, ".mcp.json"), []byte(mcpJSON), 0644)
	if err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}

	cfg, err := LoadMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if len(cfg.MCPServers) != 1 {
		t.Fatalf("MCPServers len = %d, want 1", len(cfg.MCPServers))
	}

	server, ok := cfg.MCPServers["test-server"]
	if !ok {
		t.Fatal("expected test-server in config")
	}
	if server.Command != "echo" {
		t.Errorf("Command = %q, want %q", server.Command, "echo")
	}
	if len(server.Args) != 1 || server.Args[0] != "hello" {
		t.Errorf("Args = %v, want [hello]", server.Args)
	}
}

func TestLoadMCPConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tmpDir, ".mcp.json"), []byte(`{invalid`), 0644)
	if err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}

	// Invalid JSON should result in nil config (silently ignored).
	cfg, err := LoadMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config for invalid JSON")
	}
}

func TestLoadMCPConfig_SSEServer(t *testing.T) {
	tmpDir := t.TempDir()

	mcpJSON := `{
		"mcpServers": {
			"remote": {
				"url": "https://mcp.example.com/sse"
			}
		}
	}`

	err := os.WriteFile(filepath.Join(tmpDir, ".mcp.json"), []byte(mcpJSON), 0644)
	if err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}

	cfg, err := LoadMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	server := cfg.MCPServers["remote"]
	if server.URL != "https://mcp.example.com/sse" {
		t.Errorf("URL = %q, want %q", server.URL, "https://mcp.example.com/sse")
	}
	if server.Command != "" {
		t.Errorf("Command should be empty for SSE server, got %q", server.Command)
	}
}

func TestLoadMCPConfig_WithEnv(t *testing.T) {
	tmpDir := t.TempDir()

	mcpJSON := `{
		"mcpServers": {
			"github": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"],
				"env": {
					"GITHUB_TOKEN": "ghp_test123"
				}
			}
		}
	}`

	err := os.WriteFile(filepath.Join(tmpDir, ".mcp.json"), []byte(mcpJSON), 0644)
	if err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}

	cfg, err := LoadMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	server := cfg.MCPServers["github"]
	if server.Env["GITHUB_TOKEN"] != "ghp_test123" {
		t.Errorf("GITHUB_TOKEN = %q, want %q", server.Env["GITHUB_TOKEN"], "ghp_test123")
	}
}

func TestLoadMCPConfig_MultipleServers(t *testing.T) {
	tmpDir := t.TempDir()

	mcpJSON := `{
		"mcpServers": {
			"server-a": {"command": "a"},
			"server-b": {"command": "b"},
			"server-c": {"url": "https://c.example.com"}
		}
	}`

	err := os.WriteFile(filepath.Join(tmpDir, ".mcp.json"), []byte(mcpJSON), 0644)
	if err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}

	cfg, err := LoadMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.MCPServers) != 3 {
		t.Errorf("MCPServers len = %d, want 3", len(cfg.MCPServers))
	}
}

func TestLoadMCPConfig_EmptyServers(t *testing.T) {
	tmpDir := t.TempDir()

	mcpJSON := `{"mcpServers": {}}`
	err := os.WriteFile(filepath.Join(tmpDir, ".mcp.json"), []byte(mcpJSON), 0644)
	if err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}

	cfg, err := LoadMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config for empty mcpServers")
	}
}
