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

import "github.com/spf13/cobra"

// NewCommand returns a new cobra command for `rad mcp`.
func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol (MCP) server for AI agent integration",
		Long: `Model Context Protocol (MCP) server that exposes Radius discovery
skills as tools for AI coding agents like GitHub Copilot.

The MCP server enables AI agents to:
- Discover infrastructure dependencies in code
- Detect deployable services
- Generate Radius application definitions
- Find matching recipes for resources`,
	}
}
