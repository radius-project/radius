// Package mcp provides a Model Context Protocol (MCP) server for AI agent integration.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Server represents an MCP server that exposes discovery skills as tools.
type Server struct {
	mu       sync.RWMutex
	handlers map[string]ToolHandler
	started  bool
}

// NewServer creates a new MCP server.
func NewServer() *Server {
	return &Server{
		handlers: make(map[string]ToolHandler),
	}
}

// ToolHandler handles invocations of an MCP tool.
type ToolHandler interface {
	// Name returns the tool name.
	Name() string

	// Description returns the tool description.
	Description() string

	// InputSchema returns the JSON schema for the tool's input.
	InputSchema() json.RawMessage

	// Handle processes a tool invocation and returns the result.
	Handle(ctx context.Context, input json.RawMessage) (interface{}, error)
}

// RegisterTool registers a tool handler with the server.
func (s *Server) RegisterTool(handler ToolHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("cannot register tools after server has started")
	}

	if _, exists := s.handlers[handler.Name()]; exists {
		return fmt.Errorf("tool %q already registered", handler.Name())
	}

	s.handlers[handler.Name()] = handler
	return nil
}

// ListTools returns the list of available tools.
func (s *Server) ListTools() []ToolDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]ToolDefinition, 0, len(s.handlers))
	for _, handler := range s.handlers {
		tools = append(tools, ToolDefinition{
			Name:        handler.Name(),
			Description: handler.Description(),
			InputSchema: handler.InputSchema(),
		})
	}
	return tools
}

// ToolDefinition describes an available MCP tool.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// InvokeTool invokes a tool by name with the given input.
func (s *Server) InvokeTool(ctx context.Context, name string, input json.RawMessage) (*ToolResult, error) {
	s.mu.RLock()
	handler, exists := s.handlers[name]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tool %q not found", name)
	}

	result, err := handler.Handle(ctx, input)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Content: result,
	}, nil
}

// ToolResult represents the result of a tool invocation.
type ToolResult struct {
	Success bool        `json:"success"`
	Content interface{} `json:"content,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// Start starts the MCP server.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("server already started")
	}

	s.started = true
	return nil
}

// Stop stops the MCP server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return fmt.Errorf("server not started")
	}

	s.started = false
	return nil
}

// IsStarted returns whether the server is running.
func (s *Server) IsStarted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}
