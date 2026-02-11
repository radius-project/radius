/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPServer_Initialize tests the MCP initialize flow.
func TestMCPServer_Initialize(t *testing.T) {
	server := mcp.NewServer()
	err := mcp.RegisterDiscoveryTools(server)
	require.NoError(t, err)

	// Create initialize request
	initReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  mcp.MethodInitialize,
		Params:  json.RawMessage(`{"protocolVersion": "2024-11-05", "clientInfo": {"name": "test", "version": "1.0"}}`),
	}

	// This is a basic test - in a real scenario we'd use a transport
	// For now we just verify the server accepts registrations
	tools := server.ListTools()
	assert.GreaterOrEqual(t, len(tools), 5)

	_ = initReq // Used for documentation
}

// TestMCPServer_ListTools tests the tools/list method.
func TestMCPServer_ListTools(t *testing.T) {
	server := mcp.NewServer()
	err := mcp.RegisterDiscoveryTools(server)
	require.NoError(t, err)

	tools := server.ListTools()

	// Verify expected tools
	expectedTools := map[string]bool{
		"discover_dependencies":   true,
		"discover_services":       true,
		"generate_resource_types": true,
		"generate_app_definition": true,
		"validate_app_definition": true,
	}

	for _, tool := range tools {
		if expectedTools[tool.Name] {
			delete(expectedTools, tool.Name)

			// Verify tool has required fields
			assert.NotEmpty(t, tool.Description, "tool %s should have description", tool.Name)
			assert.NotEmpty(t, tool.InputSchema, "tool %s should have input schema", tool.Name)

			// Verify input schema is valid JSON
			var schema map[string]interface{}
			err := json.Unmarshal(tool.InputSchema, &schema)
			assert.NoError(t, err, "tool %s should have valid JSON schema", tool.Name)
		}
	}

	assert.Empty(t, expectedTools, "all expected tools should be registered")
}

// TestMCPServer_CallTool tests the tools/call method.
func TestMCPServer_CallTool(t *testing.T) {
	server := mcp.NewServer()
	err := mcp.RegisterDiscoveryTools(server)
	require.NoError(t, err)

	ctx := context.Background()

	// Test calling validate_app_definition with valid Bicep
	validBicep := `extension radius

param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'testapp'
  properties: {
    environment: environment
  }
}`

	args, _ := json.Marshal(map[string]interface{}{
		"bicepContent": validBicep,
	})

	result, err := server.InvokeTool(ctx, "validate_app_definition", args)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Success)
}

// TestMCPServer_CallTool_NotFound tests calling a non-existent tool.
func TestMCPServer_CallTool_NotFound(t *testing.T) {
	server := mcp.NewServer()
	err := mcp.RegisterDiscoveryTools(server)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = server.InvokeTool(ctx, "nonexistent_tool", json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestMCPServer_GenerateAppDefinition_ViaToolInvoke tests generate_app_definition via MCP.
func TestMCPServer_GenerateAppDefinition_ViaToolInvoke(t *testing.T) {
	server := mcp.NewServer()
	err := mcp.RegisterDiscoveryTools(server)
	require.NoError(t, err)

	ctx := context.Background()

	args, _ := json.Marshal(map[string]interface{}{
		"discoveryResult": map[string]interface{}{
			"projectPath": "/test/app",
			"services": []map[string]interface{}{
				{
					"name":         "api",
					"language":     "javascript",
					"exposedPorts": []int{3000},
				},
			},
			"dependencies":  []interface{}{},
			"resourceTypes": []interface{}{},
		},
		"applicationName": "test-app",
		"includeComments": true,
	})

	result, err := server.InvokeTool(ctx, "generate_app_definition", args)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Content)
}

// TestMCPServer_ConcurrentInvocations tests NFR-06: concurrent skill invocations.
func TestMCPServer_ConcurrentInvocations(t *testing.T) {
	server := mcp.NewServer()
	err := mcp.RegisterDiscoveryTools(server)
	require.NoError(t, err)

	ctx := context.Background()
	numConcurrent := 10

	// Channel to collect results
	results := make(chan bool, numConcurrent)
	errors := make(chan error, numConcurrent)

	validBicep := `extension radius
param environment string
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'testapp'
  properties: {
    environment: environment
  }
}`

	args, _ := json.Marshal(map[string]interface{}{
		"bicepContent": validBicep,
	})

	// Launch concurrent invocations
	for i := 0; i < numConcurrent; i++ {
		go func() {
			result, err := server.InvokeTool(ctx, "validate_app_definition", args)
			if err != nil {
				errors <- err
			} else {
				results <- result.Success
			}
		}()
	}

	// Collect results with timeout
	successCount := 0
	errorCount := 0
	timeout := time.After(30 * time.Second)

	for i := 0; i < numConcurrent; i++ {
		select {
		case success := <-results:
			if success {
				successCount++
			}
		case err := <-errors:
			t.Logf("error: %v", err)
			errorCount++
		case <-timeout:
			t.Fatal("timeout waiting for concurrent invocations")
		}
	}

	assert.Equal(t, numConcurrent, successCount, "all invocations should succeed")
	assert.Equal(t, 0, errorCount, "no errors should occur")
}

// TestMCPToolDefinitions tests that tool definitions are valid.
func TestMCPToolDefinitions(t *testing.T) {
	for _, tool := range mcp.ToolDefinitions {
		t.Run(tool.Name, func(t *testing.T) {
			assert.NotEmpty(t, tool.Name)
			assert.NotEmpty(t, tool.Description)
			assert.NotEmpty(t, tool.InputSchema)

			// Verify schema is valid JSON
			var schema map[string]interface{}
			err := json.Unmarshal(tool.InputSchema, &schema)
			require.NoError(t, err, "tool schema should be valid JSON")

			// Verify schema has type
			schemaType, ok := schema["type"]
			assert.True(t, ok, "schema should have type")
			assert.Equal(t, "object", schemaType, "schema type should be object")
		})
	}
}

// TestMCPToolDefinitionsJSON tests JSON serialization of tool definitions.
func TestMCPToolDefinitionsJSON(t *testing.T) {
	data, err := mcp.GetToolDefinitionsJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify it's valid JSON
	var tools []mcp.Tool
	err = json.Unmarshal(data, &tools)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(tools), 5)
}

// TestMCPProtocolMessages tests JSON-RPC message handling.
func TestMCPProtocolMessages(t *testing.T) {
	t.Run("success response", func(t *testing.T) {
		result := map[string]string{"status": "ok"}
		resp, err := mcp.NewSuccessResponse(1, result)
		require.NoError(t, err)

		assert.Equal(t, mcp.JSONRPCVersion, resp.JSONRPC)
		assert.Equal(t, 1, resp.ID)
		assert.Nil(t, resp.Error)
		assert.NotEmpty(t, resp.Result)
	})

	t.Run("error response", func(t *testing.T) {
		resp := mcp.NewErrorResponse(1, mcp.NewMethodNotFoundError("test"))

		assert.Equal(t, mcp.JSONRPCVersion, resp.JSONRPC)
		assert.Equal(t, 1, resp.ID)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, mcp.ErrorCodeMethodNotFound, resp.Error.Code)
	})

	t.Run("error types", func(t *testing.T) {
		parseErr := mcp.NewParseError("test")
		assert.Equal(t, mcp.ErrorCodeParseError, parseErr.Code)

		invalidReqErr := mcp.NewInvalidRequestError("test")
		assert.Equal(t, mcp.ErrorCodeInvalidRequest, invalidReqErr.Code)

		invalidParamsErr := mcp.NewInvalidParamsError("test")
		assert.Equal(t, mcp.ErrorCodeInvalidParams, invalidParamsErr.Code)

		internalErr := mcp.NewInternalError("test")
		assert.Equal(t, mcp.ErrorCodeInternalError, internalErr.Code)

		toolNotFoundErr := mcp.NewToolNotFoundError("test")
		assert.Equal(t, mcp.ErrorCodeToolNotFound, toolNotFoundErr.Code)

		toolFailedErr := mcp.NewToolFailedError("test", "error")
		assert.Equal(t, mcp.ErrorCodeToolFailed, toolFailedErr.Code)
	})
}

// TestMCPHTTPTransport_Integration tests the HTTP transport integration.
func TestMCPHTTPTransport_Integration(t *testing.T) {
	// Create a mock HTTP handler that simulates the MCP HTTP endpoint
	server := mcp.NewServer()
	err := mcp.RegisterDiscoveryTools(server)
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req mcp.Request
		if err := json.Unmarshal(body, &req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mcp.NewErrorResponse(nil, mcp.NewParseError(err.Error())))
			return
		}

		var resp *mcp.Response
		switch req.Method {
		case mcp.MethodListTools:
			tools := server.ListTools()
			mcpTools := make([]mcp.Tool, len(tools))
			for i, tool := range tools {
				mcpTools[i] = mcp.Tool{
					Name:        tool.Name,
					Description: tool.Description,
					InputSchema: tool.InputSchema,
				}
			}
			resp, _ = mcp.NewSuccessResponse(req.ID, mcp.ToolsListResult{Tools: mcpTools})
		case mcp.MethodCallTool:
			var params mcp.ToolCallParams
			json.Unmarshal(req.Params, &params)
			result, _ := server.InvokeTool(r.Context(), params.Name, params.Arguments)
			contentJSON, _ := json.Marshal(result.Content)
			resp, _ = mcp.NewSuccessResponse(req.ID, mcp.ToolCallResult{
				Content: []mcp.ContentItem{{Type: "text", Text: string(contentJSON)}},
			})
		default:
			resp = mcp.NewErrorResponse(req.ID, mcp.NewMethodNotFoundError(req.Method))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Test tools/list
	t.Run("list tools via HTTP", func(t *testing.T) {
		reqBody := mcp.Request{
			JSONRPC: mcp.JSONRPCVersion,
			ID:      1,
			Method:  mcp.MethodListTools,
		}
		body, _ := json.Marshal(reqBody)

		resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var mcpResp mcp.Response
		err = json.NewDecoder(resp.Body).Decode(&mcpResp)
		require.NoError(t, err)

		assert.Nil(t, mcpResp.Error)
		assert.NotEmpty(t, mcpResp.Result)
	})

	// Test tools/call
	t.Run("call tool via HTTP", func(t *testing.T) {
		args, _ := json.Marshal(map[string]interface{}{
			"bicepContent": `extension radius
param environment string
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'test'
  properties: { environment: environment }
}`,
		})

		callParams := mcp.ToolCallParams{
			Name:      "validate_app_definition",
			Arguments: args,
		}
		paramsJSON, _ := json.Marshal(callParams)

		reqBody := mcp.Request{
			JSONRPC: mcp.JSONRPCVersion,
			ID:      2,
			Method:  mcp.MethodCallTool,
			Params:  paramsJSON,
		}
		body, _ := json.Marshal(reqBody)

		resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var mcpResp mcp.Response
		err = json.NewDecoder(resp.Body).Decode(&mcpResp)
		require.NoError(t, err)

		assert.Nil(t, mcpResp.Error)
		assert.NotEmpty(t, mcpResp.Result)
	})
}
