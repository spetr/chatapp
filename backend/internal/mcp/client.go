package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/spetr/chatapp/internal/config"
	"github.com/spetr/chatapp/internal/provider"
)

// Client manages MCP server connections
type Client struct {
	servers map[string]*ServerConnection
	mu      sync.RWMutex
}

// ServerConnection represents a connection to an MCP server
type ServerConnection struct {
	Name    string
	Config  config.MCPServerConfig
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	scanner *bufio.Scanner

	requestID atomic.Int64
	pending   map[int64]chan json.RawMessage
	pendingMu sync.RWMutex

	tools   []provider.Tool
	toolsMu sync.RWMutex
}

// JSON-RPC types
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP types
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
}

type ClientCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct{}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ListToolsResult struct {
	Tools []MCPTool `json:"tools"`
}

type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type CallToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func NewClient() *Client {
	return &Client{
		servers: make(map[string]*ServerConnection),
	}
}

func (c *Client) StartServer(ctx context.Context, cfg config.MCPServerConfig) error {
	if !cfg.Enabled {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already running
	if _, exists := c.servers[cfg.Name]; exists {
		return nil
	}

	conn := &ServerConnection{
		Name:    cfg.Name,
		Config:  cfg,
		pending: make(map[int64]chan json.RawMessage),
	}

	// Start the process
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	conn.cmd = cmd
	conn.stdin = stdin
	conn.stdout = stdout
	conn.scanner = bufio.NewScanner(stdout)
	conn.scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// Start reading responses
	go conn.readResponses()

	// Initialize the connection
	if err := conn.initialize(ctx); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("failed to initialize MCP server: %w", err)
	}

	// Get available tools
	if err := conn.refreshTools(ctx); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("failed to get tools: %w", err)
	}

	c.servers[cfg.Name] = conn
	return nil
}

func (c *Client) StopServer(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, exists := c.servers[name]
	if !exists {
		return nil
	}

	conn.stdin.Close()
	conn.cmd.Process.Kill()
	delete(c.servers, name)

	return nil
}

func (c *Client) StopAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for name, conn := range c.servers {
		conn.stdin.Close()
		conn.cmd.Process.Kill()
		delete(c.servers, name)
	}
}

func (c *Client) GetAllTools() []provider.Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var tools []provider.Tool
	for _, conn := range c.servers {
		conn.toolsMu.RLock()
		tools = append(tools, conn.tools...)
		conn.toolsMu.RUnlock()
	}
	return tools
}

// ServerStatus represents the status of an MCP server
type ServerStatus struct {
	Name      string          `json:"name"`
	Command   string          `json:"command"`
	Args      []string        `json:"args"`
	Connected bool            `json:"connected"`
	Tools     []provider.Tool `json:"tools"`
	ToolCount int             `json:"tool_count"`
}

// MCPStatus represents the overall MCP status
type MCPStatus struct {
	Enabled     bool           `json:"enabled"`
	ServerCount int            `json:"server_count"`
	TotalTools  int            `json:"total_tools"`
	Servers     []ServerStatus `json:"servers"`
}

// GetStatus returns the current MCP status
func (c *Client) GetStatus() MCPStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := MCPStatus{
		Enabled:     len(c.servers) > 0,
		ServerCount: len(c.servers),
		Servers:     make([]ServerStatus, 0, len(c.servers)),
	}

	for _, conn := range c.servers {
		conn.toolsMu.RLock()
		serverStatus := ServerStatus{
			Name:      conn.Name,
			Command:   conn.Config.Command,
			Args:      conn.Config.Args,
			Connected: conn.cmd != nil && conn.cmd.ProcessState == nil,
			Tools:     conn.tools,
			ToolCount: len(conn.tools),
		}
		status.TotalTools += len(conn.tools)
		conn.toolsMu.RUnlock()

		status.Servers = append(status.Servers, serverStatus)
	}

	return status
}

func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Find which server has this tool
	for _, conn := range c.servers {
		conn.toolsMu.RLock()
		for _, tool := range conn.tools {
			if tool.Name == name {
				conn.toolsMu.RUnlock()
				return conn.callTool(ctx, name, arguments)
			}
		}
		conn.toolsMu.RUnlock()
	}

	return "", fmt.Errorf("tool not found: %s", name)
}

func (conn *ServerConnection) readResponses() {
	for conn.scanner.Scan() {
		line := conn.scanner.Text()

		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}

		conn.pendingMu.RLock()
		ch, exists := conn.pending[resp.ID]
		conn.pendingMu.RUnlock()

		if exists {
			if resp.Error != nil {
				ch <- nil
			} else {
				ch <- resp.Result
			}
		}
	}
}

func (conn *ServerConnection) sendRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := conn.requestID.Add(1)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Create response channel
	ch := make(chan json.RawMessage, 1)
	conn.pendingMu.Lock()
	conn.pending[id] = ch
	conn.pendingMu.Unlock()

	defer func() {
		conn.pendingMu.Lock()
		delete(conn.pending, id)
		conn.pendingMu.Unlock()
	}()

	// Send request
	if _, err := conn.stdin.Write(append(data, '\n')); err != nil {
		return nil, err
	}

	// Wait for response
	select {
	case result := <-ch:
		if result == nil {
			return nil, fmt.Errorf("RPC error")
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (conn *ServerConnection) initialize(ctx context.Context) error {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: ClientCapabilities{
			Tools: &ToolsCapability{},
		},
		ClientInfo: ClientInfo{
			Name:    "chatapp",
			Version: "1.0.0",
		},
	}

	result, err := conn.sendRequest(ctx, "initialize", params)
	if err != nil {
		return err
	}

	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		return err
	}

	// Send initialized notification
	notification := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	data, _ := json.Marshal(notification)
	conn.stdin.Write(append(data, '\n'))

	return nil
}

func (conn *ServerConnection) refreshTools(ctx context.Context) error {
	result, err := conn.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return err
	}

	var toolsResult ListToolsResult
	if err := json.Unmarshal(result, &toolsResult); err != nil {
		return err
	}

	conn.toolsMu.Lock()
	conn.tools = make([]provider.Tool, len(toolsResult.Tools))
	for i, t := range toolsResult.Tools {
		conn.tools[i] = provider.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	conn.toolsMu.Unlock()

	return nil
}

func (conn *ServerConnection) callTool(ctx context.Context, name string, arguments map[string]interface{}) (string, error) {
	params := CallToolParams{
		Name:      name,
		Arguments: arguments,
	}

	result, err := conn.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return "", err
	}

	var callResult CallToolResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		return "", err
	}

	// Combine all text content
	var text string
	for _, content := range callResult.Content {
		if content.Type == "text" {
			text += content.Text
		}
	}

	if callResult.IsError {
		return "", fmt.Errorf("tool error: %s", text)
	}

	return text, nil
}
