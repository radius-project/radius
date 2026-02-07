// Package mcp provides a Model Context Protocol (MCP) server for AI agent integration.
package mcp

import (
	"encoding/json"
	"fmt"
)

// JSON-RPC 2.0 message types for MCP protocol.

const (
	// JSONRPCVersion is the JSON-RPC version.
	JSONRPCVersion = "2.0"
)

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Notification represents a JSON-RPC 2.0 notification (request without ID).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Error represents a JSON-RPC 2.0 error.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
const (
	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternalError  = -32603
)

// MCP-specific error codes.
const (
	ErrorCodeToolNotFound = -32000
	ErrorCodeToolFailed   = -32001
)

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// NewParseError creates a parse error.
func NewParseError(msg string) *Error {
	return &Error{
		Code:    ErrorCodeParseError,
		Message: msg,
	}
}

// NewInvalidRequestError creates an invalid request error.
func NewInvalidRequestError(msg string) *Error {
	return &Error{
		Code:    ErrorCodeInvalidRequest,
		Message: msg,
	}
}

// NewMethodNotFoundError creates a method not found error.
func NewMethodNotFoundError(method string) *Error {
	return &Error{
		Code:    ErrorCodeMethodNotFound,
		Message: fmt.Sprintf("method not found: %s", method),
	}
}

// NewInvalidParamsError creates an invalid params error.
func NewInvalidParamsError(msg string) *Error {
	return &Error{
		Code:    ErrorCodeInvalidParams,
		Message: msg,
	}
}

// NewInternalError creates an internal error.
func NewInternalError(msg string) *Error {
	return &Error{
		Code:    ErrorCodeInternalError,
		Message: msg,
	}
}

// NewToolNotFoundError creates a tool not found error.
func NewToolNotFoundError(tool string) *Error {
	return &Error{
		Code:    ErrorCodeToolNotFound,
		Message: fmt.Sprintf("tool not found: %s", tool),
	}
}

// NewToolFailedError creates a tool failed error.
func NewToolFailedError(tool, msg string) *Error {
	return &Error{
		Code:    ErrorCodeToolFailed,
		Message: fmt.Sprintf("tool %s failed: %s", tool, msg),
	}
}

// MCP Protocol Methods.
const (
	MethodInitialize     = "initialize"
	MethodShutdown       = "shutdown"
	MethodListTools      = "tools/list"
	MethodCallTool       = "tools/call"
	MethodListPrompts    = "prompts/list"
	MethodGetPrompt      = "prompts/get"
	MethodListResources  = "resources/list"
	MethodReadResource   = "resources/read"
)

// InitializeParams contains parameters for the initialize method.
type InitializeParams struct {
	ProtocolVersion string            `json:"protocolVersion"`
	ClientInfo      ClientInfo        `json:"clientInfo"`
	Capabilities    ClientCapabilities `json:"capabilities,omitempty"`
}

// ClientInfo describes the MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities describes client capabilities.
type ClientCapabilities struct {
	Experimental map[string]interface{} `json:"experimental,omitempty"`
}

// InitializeResult contains the result of initialization.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
}

// ServerInfo describes the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities describes server capabilities.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

// ToolsCapability describes tool capabilities.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability describes prompt capabilities.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability describes resource capabilities.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolsListResult contains the result of tools/list.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// Tool describes an MCP tool.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolCallParams contains parameters for tools/call.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolCallResult contains the result of tools/call.
type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem represents content in a tool result.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// NewSuccessResponse creates a success response.
func NewSuccessResponse(id interface{}, result interface{}) (*Response, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}

	return &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  resultJSON,
	}, nil
}

// NewErrorResponse creates an error response.
func NewErrorResponse(id interface{}, err *Error) *Response {
	return &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   err,
	}
}
