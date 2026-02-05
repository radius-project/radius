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

package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	server := NewServer()
	require.NotNil(t, server)
	assert.False(t, server.IsStarted())
}

func TestServer_RegisterTool(t *testing.T) {
	server := NewServer()

	handler := &mockToolHandler{
		name:        "test_tool",
		description: "A test tool",
		inputSchema: json.RawMessage(`{"type": "object"}`),
	}

	err := server.RegisterTool(handler)
	require.NoError(t, err)

	// Verify tool is registered
	tools := server.ListTools()
	assert.Len(t, tools, 1)
	assert.Equal(t, "test_tool", tools[0].Name)
}

func TestServer_RegisterTool_Duplicate(t *testing.T) {
	server := NewServer()

	handler := &mockToolHandler{name: "test_tool"}
	err := server.RegisterTool(handler)
	require.NoError(t, err)

	// Try to register again
	err = server.RegisterTool(handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestServer_RegisterTool_AfterStart(t *testing.T) {
	server := NewServer()

	err := server.Start()
	require.NoError(t, err)

	handler := &mockToolHandler{name: "test_tool"}
	err = server.RegisterTool(handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot register tools after server has started")
}

func TestServer_ListTools(t *testing.T) {
	server := NewServer()

	// Register multiple tools
	handlers := []*mockToolHandler{
		{name: "tool1", description: "Tool 1"},
		{name: "tool2", description: "Tool 2"},
		{name: "tool3", description: "Tool 3"},
	}

	for _, h := range handlers {
		err := server.RegisterTool(h)
		require.NoError(t, err)
	}

	tools := server.ListTools()
	assert.Len(t, tools, 3)

	// Verify all tools are present
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	assert.True(t, names["tool1"])
	assert.True(t, names["tool2"])
	assert.True(t, names["tool3"])
}

func TestServer_InvokeTool_Success(t *testing.T) {
	server := NewServer()

	handler := &mockToolHandler{
		name: "test_tool",
		handleFunc: func(ctx context.Context, input json.RawMessage) (interface{}, error) {
			return map[string]string{"result": "success"}, nil
		},
	}

	err := server.RegisterTool(handler)
	require.NoError(t, err)

	result, err := server.InvokeTool(context.Background(), "test_tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.NotNil(t, result.Content)
}

func TestServer_InvokeTool_NotFound(t *testing.T) {
	server := NewServer()

	_, err := server.InvokeTool(context.Background(), "nonexistent", json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestServer_InvokeTool_HandlerError(t *testing.T) {
	server := NewServer()

	handler := &mockToolHandler{
		name: "failing_tool",
		handleFunc: func(ctx context.Context, input json.RawMessage) (interface{}, error) {
			return nil, assert.AnError
		},
	}

	err := server.RegisterTool(handler)
	require.NoError(t, err)

	result, err := server.InvokeTool(context.Background(), "failing_tool", json.RawMessage(`{}`))
	require.NoError(t, err) // Tool invocation error is returned in result, not as error
	require.NotNil(t, result)

	assert.False(t, result.Success)
	assert.NotEmpty(t, result.Error)
}

func TestServer_StartStop(t *testing.T) {
	server := NewServer()

	// Start
	err := server.Start()
	require.NoError(t, err)
	assert.True(t, server.IsStarted())

	// Start again - should error
	err = server.Start()
	assert.Error(t, err)

	// Stop
	err = server.Stop()
	require.NoError(t, err)
	assert.False(t, server.IsStarted())

	// Stop again - should error
	err = server.Stop()
	assert.Error(t, err)
}

func TestServer_Concurrency(t *testing.T) {
	server := NewServer()

	// Register a tool that simulates some work
	handler := &mockToolHandler{
		name: "concurrent_tool",
		handleFunc: func(ctx context.Context, input json.RawMessage) (interface{}, error) {
			time.Sleep(10 * time.Millisecond)
			return map[string]string{"status": "done"}, nil
		},
	}

	err := server.RegisterTool(handler)
	require.NoError(t, err)

	// Run concurrent invocations
	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)
	results := make(chan *ToolResult, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			result, err := server.InvokeTool(context.Background(), "concurrent_tool", json.RawMessage(`{}`))
			if err != nil {
				errors <- err
			} else {
				results <- result
			}
		}()
	}

	wg.Wait()
	close(errors)
	close(results)

	// Verify no errors
	for err := range errors {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify all succeeded
	successCount := 0
	for result := range results {
		if result.Success {
			successCount++
		}
	}
	assert.Equal(t, numGoroutines, successCount)
}

func TestServer_ConcurrentToolsListAndInvoke(t *testing.T) {
	server := NewServer()

	// Register tools
	for i := 0; i < 5; i++ {
		handler := &mockToolHandler{
			name: "tool_" + string(rune('a'+i)),
			handleFunc: func(ctx context.Context, input json.RawMessage) (interface{}, error) {
				return "ok", nil
			},
		}
		err := server.RegisterTool(handler)
		require.NoError(t, err)
	}

	// Run concurrent list and invoke operations
	var wg sync.WaitGroup
	numOps := 20
	wg.Add(numOps)

	for i := 0; i < numOps; i++ {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				// List tools
				tools := server.ListTools()
				assert.Len(t, tools, 5)
			} else {
				// Invoke tool
				_, err := server.InvokeTool(context.Background(), "tool_a", json.RawMessage(`{}`))
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()
}

// mockToolHandler is a mock implementation of ToolHandler for testing.
type mockToolHandler struct {
	name        string
	description string
	inputSchema json.RawMessage
	handleFunc  func(ctx context.Context, input json.RawMessage) (interface{}, error)
}

func (h *mockToolHandler) Name() string {
	return h.name
}

func (h *mockToolHandler) Description() string {
	return h.description
}

func (h *mockToolHandler) InputSchema() json.RawMessage {
	if h.inputSchema == nil {
		return json.RawMessage(`{"type": "object"}`)
	}
	return h.inputSchema
}

func (h *mockToolHandler) Handle(ctx context.Context, input json.RawMessage) (interface{}, error) {
	if h.handleFunc != nil {
		return h.handleFunc(ctx, input)
	}
	return nil, nil
}
