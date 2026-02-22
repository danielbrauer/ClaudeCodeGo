package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// StdioTransport communicates with an MCP server subprocess via stdin/stdout.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	stderr bytes.Buffer
	mu     sync.Mutex
	done   chan struct{}
}

// NewStdioTransport starts an MCP server subprocess and returns a transport.
// The subprocess is started with the given command, args, and environment.
// The cwd parameter sets the working directory for the subprocess.
func NewStdioTransport(command string, args []string, env map[string]string, cwd string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = cwd

	// Merge environment: inherit current env, override with server-specific vars.
	cmdEnv := os.Environ()
	for k, v := range env {
		cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = cmdEnv

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	t := &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewScanner(stdout),
		done:   make(chan struct{}),
	}

	// Capture stderr for diagnostics.
	cmd.Stderr = &t.stderr

	// Increase scanner buffer for large JSON responses (10MB).
	t.stdout.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("start subprocess %q: %w", command, err)
	}

	// Monitor subprocess exit.
	go func() {
		cmd.Wait()
		close(t.done)
	}()

	return t, nil
}

// Send writes a JSON-RPC request to stdin and reads the response from stdout.
func (t *StdioTransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if process is still alive.
	select {
	case <-t.done:
		stderrStr := t.stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("subprocess exited: %s", stderrStr)
		}
		return nil, fmt.Errorf("subprocess exited")
	default:
	}

	// Marshal and write request as a single line.
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	data = append(data, '\n')
	if _, err := t.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("write to stdin: %w", err)
	}

	// Read response line from stdout, respecting context cancellation.
	type scanResult struct {
		line []byte
		err  error
	}
	resultCh := make(chan scanResult, 1)

	go func() {
		if t.stdout.Scan() {
			// Copy the bytes since the scanner reuses the buffer.
			line := make([]byte, len(t.stdout.Bytes()))
			copy(line, t.stdout.Bytes())
			resultCh <- scanResult{line: line}
		} else {
			err := t.stdout.Err()
			if err == nil {
				err = io.EOF
			}
			resultCh <- scanResult{err: err}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.err != nil {
			stderrStr := t.stderr.String()
			if stderrStr != "" {
				return nil, fmt.Errorf("read stdout: %w (stderr: %s)", result.err, stderrStr)
			}
			return nil, fmt.Errorf("read stdout: %w", result.err)
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(result.line, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w (raw: %s)", err, string(result.line))
		}

		return &resp, nil
	}
}

// Notify writes a JSON-RPC notification to stdin (no response expected).
func (t *StdioTransport) Notify(ctx context.Context, req *JSONRPCRequest) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	data = append(data, '\n')
	if _, err := t.stdin.Write(data); err != nil {
		return fmt.Errorf("write notification to stdin: %w", err)
	}

	return nil
}

// Close gracefully shuts down the subprocess.
func (t *StdioTransport) Close() error {
	// Close stdin to signal EOF to the subprocess.
	t.stdin.Close()

	// Wait for the process to exit with a timeout.
	select {
	case <-t.done:
		return nil
	case <-time.After(5 * time.Second):
		// Force kill if graceful shutdown fails.
		if t.cmd.Process != nil {
			t.cmd.Process.Kill()
		}
		<-t.done
		return nil
	}
}
