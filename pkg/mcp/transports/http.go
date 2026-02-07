// Package transports provides MCP transport implementations.
package transports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	mcp "github.com/radius-project/radius/pkg/mcp"
)

// HTTPTransport provides MCP communication over HTTP.
type HTTPTransport struct {
	server     *mcp.Server
	httpServer *http.Server
	addr       string
	mu         sync.Mutex
	started    bool
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(server *mcp.Server, addr string) *HTTPTransport {
	return &HTTPTransport{
		server: server,
		addr:   addr,
	}
}

// Start starts the HTTP server.
func (t *HTTPTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return fmt.Errorf("transport already started")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", t.handleMCP)
	mux.HandleFunc("/health", t.handleHealth)

	t.httpServer = &http.Server{
		Addr:         t.addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	t.started = true
	t.mu.Unlock()

	go func() {
		<-ctx.Done()
		t.Stop()
	}()

	return t.httpServer.ListenAndServe()
}

// Stop stops the HTTP server.
func (t *HTTPTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.started {
		return nil
	}

	t.started = false
	if t.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return t.httpServer.Shutdown(ctx)
	}
	return nil
}

func (t *HTTPTransport) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.sendError(w, nil, mcp.NewParseError(err.Error()))
		return
	}
	defer r.Body.Close()

	var req mcp.Request
	if err := json.Unmarshal(body, &req); err != nil {
		t.sendError(w, nil, mcp.NewParseError(err.Error()))
		return
	}

	response := t.processRequest(r.Context(), &req)
	t.sendResponse(w, response)
}

func (t *HTTPTransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (t *HTTPTransport) processRequest(ctx context.Context, req *mcp.Request) *mcp.Response {
	switch req.Method {
	case mcp.MethodInitialize:
		return t.handleInitialize(req)
	case mcp.MethodShutdown:
		return t.handleShutdown(req)
	case mcp.MethodListTools:
		return t.handleListTools(req)
	case mcp.MethodCallTool:
		return t.handleCallTool(ctx, req)
	default:
		return mcp.NewErrorResponse(req.ID, mcp.NewMethodNotFoundError(req.Method))
	}
}

func (t *HTTPTransport) handleInitialize(req *mcp.Request) *mcp.Response {
	result := mcp.InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo: mcp.ServerInfo{
			Name:    "radius-discovery",
			Version: "1.0.0",
		},
		Capabilities: mcp.ServerCapabilities{
			Tools: &mcp.ToolsCapability{
				ListChanged: false,
			},
		},
	}

	resp, err := mcp.NewSuccessResponse(req.ID, result)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError(err.Error()))
	}
	return resp
}

func (t *HTTPTransport) handleShutdown(req *mcp.Request) *mcp.Response {
	resp, err := mcp.NewSuccessResponse(req.ID, nil)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError(err.Error()))
	}
	return resp
}

func (t *HTTPTransport) handleListTools(req *mcp.Request) *mcp.Response {
	tools := t.server.ListTools()

	mcpTools := make([]mcp.Tool, len(tools))
	for i, tool := range tools {
		mcpTools[i] = mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}
	}

	result := mcp.ToolsListResult{
		Tools: mcpTools,
	}

	resp, err := mcp.NewSuccessResponse(req.ID, result)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError(err.Error()))
	}
	return resp
}

func (t *HTTPTransport) handleCallTool(ctx context.Context, req *mcp.Request) *mcp.Response {
	var params mcp.ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError(err.Error()))
	}

	toolResult, err := t.server.InvokeTool(ctx, params.Name, params.Arguments)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewToolNotFoundError(params.Name))
	}

	var result mcp.ToolCallResult
	if toolResult.Success {
		contentJSON, _ := json.Marshal(toolResult.Content)
		result = mcp.ToolCallResult{
			Content: []mcp.ContentItem{
				{
					Type: "text",
					Text: string(contentJSON),
				},
			},
			IsError: false,
		}
	} else {
		result = mcp.ToolCallResult{
			Content: []mcp.ContentItem{
				{
					Type: "text",
					Text: toolResult.Error,
				},
			},
			IsError: true,
		}
	}

	resp, err := mcp.NewSuccessResponse(req.ID, result)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError(err.Error()))
	}
	return resp
}

func (t *HTTPTransport) sendResponse(w http.ResponseWriter, resp *mcp.Response) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (t *HTTPTransport) sendError(w http.ResponseWriter, id interface{}, err *mcp.Error) {
	t.sendResponse(w, mcp.NewErrorResponse(id, err))
}
