package mcp

import (
	"testing"
)

func TestManager_Servers_Empty(t *testing.T) {
	m := NewManager("/tmp")

	servers := m.Servers()
	if len(servers) != 0 {
		t.Errorf("Servers() = %v, want empty", servers)
	}
}

func TestManager_Client_NotFound(t *testing.T) {
	m := NewManager("/tmp")

	_, ok := m.Client("nonexistent")
	if ok {
		t.Error("expected Client to return false for nonexistent server")
	}
}

func TestManager_ServerStatus_NotConnected(t *testing.T) {
	m := NewManager("/tmp")

	status := m.ServerStatus("nonexistent")
	if status != "nonexistent: not connected" {
		t.Errorf("ServerStatus = %q", status)
	}
}

func TestManager_TransportForConfig_StdioNoCommand(t *testing.T) {
	m := NewManager("/tmp")

	_, err := m.transportForConfig(ServerConfig{})
	if err == nil {
		t.Error("expected error for config with no url or command")
	}
}

func TestManager_TransportForConfig_SSE(t *testing.T) {
	m := NewManager("/tmp")

	transport, err := m.transportForConfig(ServerConfig{URL: "https://example.com/sse"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := transport.(*SSETransport); !ok {
		t.Error("expected SSETransport for URL config")
	}

	transport.Close()
}

func TestManager_Shutdown_Empty(t *testing.T) {
	m := NewManager("/tmp")
	// Should not panic.
	m.Shutdown()
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		strs []string
		sep  string
		want string
	}{
		{nil, ", ", ""},
		{[]string{"a"}, ", ", "a"},
		{[]string{"a", "b", "c"}, ", ", "a, b, c"},
		{[]string{"tools", "resources"}, " | ", "tools | resources"},
	}

	for _, tt := range tests {
		got := joinStrings(tt.strs, tt.sep)
		if got != tt.want {
			t.Errorf("joinStrings(%v, %q) = %q, want %q", tt.strs, tt.sep, got, tt.want)
		}
	}
}
