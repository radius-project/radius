// Package mcp provides a Model Context Protocol (MCP) server for AI agent integration.
package mcp

import (
	"encoding/json"
)

// ToolDefinitions contains the MCP tool definitions for discovery skills.
var ToolDefinitions = []Tool{
	{
		Name:        "discover_dependencies",
		Description: "Analyze a codebase to detect infrastructure dependencies such as databases, caches, and message queues",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"projectPath": {
					"type": "string",
					"description": "Path to the project directory to analyze"
				},
				"includeDevDependencies": {
					"type": "boolean",
					"description": "Whether to include development dependencies",
					"default": false
				},
				"minConfidence": {
					"type": "number",
					"description": "Minimum confidence threshold (0.0-1.0)",
					"default": 0.5
				}
			},
			"required": ["projectPath"]
		}`),
	},
	{
		Name:        "discover_services",
		Description: "Detect deployable services in a codebase based on Dockerfiles, package manifests, and entrypoints",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"projectPath": {
					"type": "string",
					"description": "Path to the project directory to analyze"
				}
			},
			"required": ["projectPath"]
		}`),
	},
	{
		Name:        "discover_team_practices",
		Description: "Extract team infrastructure practices from existing IaC files (Terraform, Bicep, Helm)",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"projectPath": {
					"type": "string",
					"description": "Path to the project directory to analyze"
				}
			},
			"required": ["projectPath"]
		}`),
	},
	{
		Name:        "generate_resource_types",
		Description: "Map detected dependencies to Radius resource types for infrastructure provisioning",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"dependencies": {
					"type": "array",
					"description": "List of detected dependencies to map",
					"items": {
						"type": "object",
						"properties": {
							"id": {"type": "string"},
							"type": {"type": "string"},
							"library": {"type": "string"}
						}
					}
				}
			},
			"required": ["dependencies"]
		}`),
	},
	{
		Name:        "discover_recipes",
		Description: "Match discovered infrastructure dependencies to available Radius recipes from various sources",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"resourceTypeMappings": {
					"type": "array",
					"description": "Resource type mappings from generate_resource_types",
					"items": {
						"type": "object",
						"properties": {
							"dependencyId": {"type": "string"},
							"dependencyName": {"type": "string"},
							"resourceType": {
								"type": "object",
								"properties": {
									"name": {"type": "string"},
									"provider": {"type": "string"}
								}
							},
							"confidence": {"type": "number"}
						}
					}
				},
				"cloudProvider": {
					"type": "string",
					"description": "Filter recipes by cloud provider (aws, azure, gcp)"
				},
				"minConfidence": {
					"type": "number",
					"description": "Minimum confidence threshold for matches",
					"default": 0.3
				},
				"maxMatchesPerType": {
					"type": "integer",
					"description": "Maximum recipe matches per resource type",
					"default": 5
				},
				"preferredSources": {
					"type": "array",
					"description": "Preferred recipe sources in priority order",
					"items": {"type": "string"}
				}
			},
			"required": ["resourceTypeMappings"]
		}`),
	},
	{
		Name:        "generate_app_definition",
		Description: "Generate a Radius application definition (app.bicep) from discovery results",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"discoveryResult": {
					"type": "object",
					"description": "The discovery result containing services, dependencies, and resource types"
				},
				"applicationName": {
					"type": "string",
					"description": "Name for the Radius application"
				},
				"environment": {
					"type": "string",
					"description": "Target Radius environment"
				},
				"outputPath": {
					"type": "string",
					"description": "Path to write the generated app.bicep"
				},
				"includeComments": {
					"type": "boolean",
					"description": "Include helpful comments in generated Bicep",
					"default": true
				}
			},
			"required": ["discoveryResult"]
		}`),
	},
	{
		Name:        "validate_app_definition",
		Description: "Validate a generated Radius application definition (app.bicep) for correctness",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"bicepContent": {
					"type": "string",
					"description": "The Bicep content to validate"
				},
				"filePath": {
					"type": "string",
					"description": "Path to app.bicep file to validate"
				},
				"discoveryResult": {
					"type": "object",
					"description": "Discovery result for cross-reference validation"
				},
				"strictMode": {
					"type": "boolean",
					"description": "Enable additional validation checks",
					"default": false
				}
			}
		}`),
	},
}

// GetToolDefinitionsJSON returns the tool definitions as JSON.
func GetToolDefinitionsJSON() ([]byte, error) {
	return json.MarshalIndent(ToolDefinitions, "", "  ")
}
