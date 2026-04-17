package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Guardian manages the mcp-guardian child process over stdio.
type Guardian struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	id     int
}

// NewGuardian creates a Guardian that spawns mcp-guardian as a child process.
func NewGuardian(binaryPath, configPath string) *Guardian {
	args := []string{}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	return &Guardian{
		cmd: exec.Command(binaryPath, args...),
	}
}

// Start launches the mcp-guardian process.
func (g *Guardian) Start() error {
	var err error
	g.stdin, err = g.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := g.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	g.stdout = bufio.NewReader(stdoutPipe)

	if err := g.cmd.Start(); err != nil {
		return fmt.Errorf("start guardian: %w", err)
	}
	return nil
}

// Stop terminates the mcp-guardian process.
func (g *Guardian) Stop() error {
	if g.stdin != nil {
		g.stdin.Close()
	}
	if g.cmd.Process != nil {
		return g.cmd.Process.Kill()
	}
	return nil
}

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Call sends a JSON-RPC request and returns the response.
func (g *Guardian) Call(method string, params interface{}) (*Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.id++
	req := Request{
		JSONRPC: "2.0",
		ID:      g.id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if _, err := fmt.Fprintf(g.stdin, "%s\n", data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	line, err := g.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return &resp, nil
}
