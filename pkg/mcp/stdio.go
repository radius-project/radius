// Package mcp provides a Model Context Protocol (MCP) server for AI agent integration.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// StdioTransport provides MCP communication over stdin/stdout.
type StdioTransport struct {
	server  *Server
	reader  *bufio.Reader
	writer  io.Writer
	mu      sync.Mutex
	started bool
	done    chan struct{}
}

// NewStdioTransport creates a new stdio transport.
func NewStdioTransport(server *Server) *StdioTransport {
	return &StdioTransport{
		server: server,
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
		done:   make(chan struct{}),
	}
}

// Start starts the stdio transport, reading from stdin and writing to stdout.
func (t *StdioTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return fmt.Errorf("transport already started")
	}
	t.started = true
	t.mu.Unlock()

	go t.readLoop(ctx)
	return nil
}

// Stop stops the stdio transport.
func (t *StdioTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.started {
		return nil
	}

	t.started = false
	close(t.done)
	return nil
}

func (t *StdioTransport) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		default:
			line, err := t.reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					return
				}
				continue
			}

			if len(line) == 0 {
				continue
			}

			t.handleMessage(ctx, line)
		}
	}
}

func (t *StdioTransport) handleMessage(ctx context.Context, data []byte) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		t.sendError(nil, NewParseError(err.Error()))
		return
	}

	response := t.processRequest(ctx, &req)
	t.sendResponse(response)
}

func (t *StdioTransport) processRequest(ctx context.Context, req *Request) *Response {
	switch req.Method {
	case MethodInitialize:
		return t.handleInitialize(req)
	case MethodShutdown:
		return t.handleShutdown(req)
	case MethodListTools:
		return t.handleListTools(req)
	case MethodCallTool:
		return t.handleCallTool(ctx, req)
	default:
		return NewErrorResponse(req.ID, NewMethodNotFoundError(req.Method))
	}
}

func (t *StdioTransport) handleInitialize(req *Request) *Response {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo: ServerInfo{
			Name:    "radius-discovery",
			Version: "1.0.0",
		},
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
		},
	}

	resp, err := NewSuccessResponse(req.ID, result)
	if err != nil {
		return NewErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return resp
}

func (t *StdioTransport) handleShutdown(req *Request) *Response {
	resp, err := NewSuccessResponse(req.ID, nil)
	if err != nil {
		return NewErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return resp
}

func (t *StdioTransport) handleListTools(req *Request) *Response {
	tools := t.server.ListTools()

	mcpTools := make([]Tool, len(tools))
	for i, tool := range tools {
		mcpTools[i] = Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}
	}

	result := ToolsListResult{
		Tools: mcpTools,
	}

	resp, err := NewSuccessResponse(req.ID, result)
	if err != nil {
		return NewErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return resp
}

func (t *StdioTransport) handleCallTool(ctx context.Context, req *Request) *Response {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}

	toolResult, err := t.server.InvokeTool(ctx, params.Name, params.Arguments)
	if err != nil {
		return NewErrorResponse(req.ID, NewToolNotFoundError(params.Name))
	}

	var result ToolCallResult
	if toolResult.Success {
		contentJSON, _ := json.Marshal(toolResult.Content)
		result = ToolCallResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(contentJSON),
				},
			},
			IsError: false,
		}
	} else {
		result = ToolCallResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: toolResult.Error,
				},
			},
			IsError: true,
		}
	}

	resp, err := NewSuccessResponse(req.ID, result)
	if err != nil {
		return NewErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return resp
}

func (t *StdioTransport) sendResponse(resp *Response) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	data = append(data, '\n')
	t.writer.Write(data) //nolint:errcheck
}

func (t *StdioTransport) sendError(id interface{}, err *Error) {
	t.sendResponse(NewErrorResponse(id, err))
}
