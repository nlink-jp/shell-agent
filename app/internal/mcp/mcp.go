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
	stdout *bufio.Scanner
	mu     sync.Mutex
	id     int
	tools  []ToolDef
}

// ToolDef represents an MCP tool definition.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// NewGuardian creates a Guardian that spawns mcp-guardian as a child process.
func NewGuardian(binaryPath string, args ...string) *Guardian {
	return &Guardian{
		cmd: exec.Command(binaryPath, args...),
	}
}

// Start launches the mcp-guardian process and initializes the MCP session.
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
	g.stdout = bufio.NewScanner(stdoutPipe)
	g.stdout.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer

	if err := g.cmd.Start(); err != nil {
		return fmt.Errorf("start guardian: %w", err)
	}

	// Initialize MCP session
	if err := g.initialize(); err != nil {
		g.Stop()
		return fmt.Errorf("MCP initialize: %w", err)
	}

	// Fetch available tools
	if err := g.refreshTools(); err != nil {
		g.Stop()
		return fmt.Errorf("MCP tools/list: %w", err)
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

// Tools returns the cached list of available MCP tools.
func (g *Guardian) Tools() []ToolDef {
	return g.tools
}

// CallTool invokes an MCP tool via guardian.
func (g *Guardian) CallTool(name string, arguments json.RawMessage) (json.RawMessage, error) {
	resp, err := g.call("tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
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

func (g *Guardian) initialize() error {
	_, err := g.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "shell-agent",
			"version": "0.1.0",
		},
	})
	return err
}

func (g *Guardian) refreshTools() error {
	resp, err := g.call("tools/list", nil)
	if err != nil {
		return err
	}

	var result struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parse tools: %w", err)
	}
	g.tools = result.Tools
	return nil
}

func (g *Guardian) call(method string, params any) (*Response, error) {
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

	if !g.stdout.Scan() {
		if err := g.stdout.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, fmt.Errorf("read response: EOF")
	}

	var resp Response
	if err := json.Unmarshal(g.stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return &resp, nil
}
