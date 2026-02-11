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
	"os"
	"os/signal"
	"syscall"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	rapmcp "github.com/radius-project/radius/pkg/mcp"
	"github.com/radius-project/radius/pkg/mcp/transports"
	"github.com/spf13/cobra"
)

const (
	flagTransport = "transport"
	flagAddress   = "address"
)

// NewServeCommand creates an instance of the `rad mcp serve` command and runner.
func NewServeCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server for AI agent integration",
		Long: `Starts the Model Context Protocol (MCP) server that exposes Radius discovery
skills as tools for AI coding agents.

The MCP server can be used with:
- VS Code GitHub Copilot extensions (stdio transport)
- Remote AI agents (HTTP transport)

Available tools:
- discover_dependencies: Detect infrastructure dependencies in code
- discover_services: Detect deployable services
- discover_team_practices: Extract team practices from IaC files
- generate_resource_types: Map dependencies to Radius resource types
- discover_recipes: Find matching Radius recipes
- generate_app_definition: Generate app.bicep from discovery
- validate_app_definition: Validate generated Bicep`,
		Example: `
# Start MCP server with stdio transport (for VS Code)
rad mcp serve

# Start MCP server with HTTP transport
rad mcp serve --transport http --address :8080
`,
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().String(flagTransport, "stdio", "Transport type (stdio, http)")
	cmd.Flags().String(flagAddress, ":8080", "Address for HTTP transport")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad mcp serve` command.
type Runner struct {
	Output output.Interface

	Transport string
	Address   string
}

// NewRunner creates an instance of the runner for the `rad mcp serve` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Output: factory.GetOutput(),
	}
}

// Validate implements the framework.Runner interface.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	transport, _ := cmd.Flags().GetString(flagTransport)
	address, _ := cmd.Flags().GetString(flagAddress)

	r.Transport = transport
	r.Address = address

	return nil
}

// Run implements the framework.Runner interface.
func (r *Runner) Run(ctx context.Context) error {
	// Create MCP server
	server := rapmcp.NewServer()

	// Register discovery tools
	if err := rapmcp.RegisterDiscoveryTools(server); err != nil {
		return err
	}

	// Start server
	if err := server.Start(); err != nil {
		return err
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-sigChan
		cancel()
	}()

	// Start transport
	switch r.Transport {
	case "stdio":
		r.Output.LogInfo("Starting MCP server with stdio transport...")
		transport := rapmcp.NewStdioTransport(server)
		return transport.Start(ctx)
	case "http":
		r.Output.LogInfo("Starting MCP server with HTTP transport on %s...", r.Address)
		transport := transports.NewHTTPTransport(server, r.Address)
		return transport.Start(ctx)
	default:
		r.Output.LogInfo("Unknown transport: %s, using stdio", r.Transport)
		transport := rapmcp.NewStdioTransport(server)
		return transport.Start(ctx)
	}
}
