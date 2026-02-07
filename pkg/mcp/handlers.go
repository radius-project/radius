// Package mcp provides a Model Context Protocol (MCP) server for AI agent integration.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/skills"
)

// RegisterDiscoveryTools registers all discovery skills as MCP tools.
func RegisterDiscoveryTools(server *Server) error {
	handlers := []ToolHandler{
		NewDiscoverDependenciesHandler(),
		NewDiscoverServicesHandler(),
		NewGenerateResourceTypesHandler(),
		NewDiscoverRecipesHandler(),
		NewGenerateAppDefinitionHandler(),
		NewValidateAppDefinitionHandler(),
	}

	for _, handler := range handlers {
		if err := server.RegisterTool(handler); err != nil {
			return fmt.Errorf("registering tool %s: %w", handler.Name(), err)
		}
	}

	return nil
}

// DiscoverDependenciesHandler handles the discover_dependencies tool.
type DiscoverDependenciesHandler struct {
	skill *skills.DiscoverDependenciesSkill
}

// NewDiscoverDependenciesHandler creates a new handler.
func NewDiscoverDependenciesHandler() *DiscoverDependenciesHandler {
	// Use the default skill from the registry or create one with defaults
	skill := skills.NewDiscoverDependenciesSkillWithDefaults()

	return &DiscoverDependenciesHandler{
		skill: skill,
	}
}

func (h *DiscoverDependenciesHandler) Name() string {
	return "discover_dependencies"
}

func (h *DiscoverDependenciesHandler) Description() string {
	return "Analyze a codebase to detect infrastructure dependencies such as databases, caches, and message queues"
}

func (h *DiscoverDependenciesHandler) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"projectPath": {
				"type": "string",
				"description": "Path to the project directory to analyze"
			},
			"includeDevDeps": {
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
	}`)
}

func (h *DiscoverDependenciesHandler) Handle(ctx context.Context, input json.RawMessage) (interface{}, error) {
	var params struct {
		ProjectPath    string  `json:"projectPath"`
		IncludeDevDeps bool    `json:"includeDevDeps"`
		MinConfidence  float64 `json:"minConfidence"`
	}

	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if params.ProjectPath == "" {
		return nil, fmt.Errorf("projectPath is required")
	}

	if params.MinConfidence == 0 {
		params.MinConfidence = 0.5
	}

	skillInput := skills.SkillInput{
		ProjectPath: params.ProjectPath,
		Parameters: map[string]interface{}{
			"minConfidence":  params.MinConfidence,
			"includeDevDeps": params.IncludeDevDeps,
		},
	}

	result, err := h.skill.Execute(ctx, skillInput)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// DiscoverServicesHandler handles the discover_services tool.
type DiscoverServicesHandler struct {
	skill *skills.DiscoverServicesSkill
}

// NewDiscoverServicesHandler creates a new handler.
func NewDiscoverServicesHandler() *DiscoverServicesHandler {
	return &DiscoverServicesHandler{
		skill: skills.NewDiscoverServicesSkill(),
	}
}

func (h *DiscoverServicesHandler) Name() string {
	return "discover_services"
}

func (h *DiscoverServicesHandler) Description() string {
	return "Detect deployable services in a codebase based on Dockerfiles, package manifests, and entrypoints"
}

func (h *DiscoverServicesHandler) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"projectPath": {
				"type": "string",
				"description": "Path to the project directory to analyze"
			}
		},
		"required": ["projectPath"]
	}`)
}

func (h *DiscoverServicesHandler) Handle(ctx context.Context, input json.RawMessage) (interface{}, error) {
	var params struct {
		ProjectPath string `json:"projectPath"`
	}

	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if params.ProjectPath == "" {
		return nil, fmt.Errorf("projectPath is required")
	}

	skillInput := skills.SkillInput{
		ProjectPath: params.ProjectPath,
	}

	result, err := h.skill.Execute(ctx, skillInput)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GenerateResourceTypesHandler handles the generate_resource_types tool.
type GenerateResourceTypesHandler struct {
	skill *skills.GenerateResourceTypesSkill
}

// NewGenerateResourceTypesHandler creates a new handler.
func NewGenerateResourceTypesHandler() *GenerateResourceTypesHandler {
	skill := skills.NewGenerateResourceTypesSkillWithDefaults()

	return &GenerateResourceTypesHandler{
		skill: skill,
	}
}

func (h *GenerateResourceTypesHandler) Name() string {
	return "generate_resource_types"
}

func (h *GenerateResourceTypesHandler) Description() string {
	return "Map detected dependencies to Radius resource types for infrastructure provisioning"
}

func (h *GenerateResourceTypesHandler) InputSchema() json.RawMessage {
	return json.RawMessage(`{
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
						"name": {"type": "string"},
						"library": {"type": "string"},
						"confidence": {"type": "number"}
					}
				}
			}
		},
		"required": ["dependencies"]
	}`)
}

func (h *GenerateResourceTypesHandler) Handle(ctx context.Context, input json.RawMessage) (interface{}, error) {
	var params struct {
		Dependencies []map[string]interface{} `json:"dependencies"`
	}

	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if len(params.Dependencies) == 0 {
		return nil, fmt.Errorf("dependencies is required")
	}

	// Convert to interface slice for context
	depsInterface := make([]interface{}, len(params.Dependencies))
	for i, d := range params.Dependencies {
		depsInterface[i] = d
	}

	skillInput := skills.SkillInput{
		Context: &skills.SkillContext{
			Dependencies: depsInterface,
		},
	}

	result, err := h.skill.Execute(ctx, skillInput)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GenerateAppDefinitionHandler handles the generate_app_definition tool.
type GenerateAppDefinitionHandler struct{}

// NewGenerateAppDefinitionHandler creates a new handler.
func NewGenerateAppDefinitionHandler() *GenerateAppDefinitionHandler {
	return &GenerateAppDefinitionHandler{}
}

func (h *GenerateAppDefinitionHandler) Name() string {
	return "generate_app_definition"
}

func (h *GenerateAppDefinitionHandler) Description() string {
	return "Generate a Radius application definition (app.bicep) from discovery results"
}

func (h *GenerateAppDefinitionHandler) InputSchema() json.RawMessage {
	return json.RawMessage(`{
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
	}`)
}

func (h *GenerateAppDefinitionHandler) Handle(ctx context.Context, input json.RawMessage) (interface{}, error) {
	var params struct {
		DiscoveryResult *discovery.DiscoveryResult `json:"discoveryResult"`
		ApplicationName string                     `json:"applicationName"`
		Environment     string                     `json:"environment"`
		OutputPath      string                     `json:"outputPath"`
		IncludeComments bool                       `json:"includeComments"`
	}

	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if params.DiscoveryResult == nil {
		return nil, fmt.Errorf("discoveryResult is required")
	}

	skill, err := skills.NewGenerateAppDefinitionSkill()
	if err != nil {
		return nil, fmt.Errorf("creating skill: %w", err)
	}

	skillInput := &skills.GenerateAppDefinitionInput{
		DiscoveryResult: params.DiscoveryResult,
		ApplicationName: params.ApplicationName,
		Environment:     params.Environment,
		OutputPath:      params.OutputPath,
		IncludeComments: params.IncludeComments,
	}

	result, err := skill.Execute(skillInput)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ValidateAppDefinitionHandler handles the validate_app_definition tool.
type ValidateAppDefinitionHandler struct {
	skill *skills.ValidateAppDefinitionSkill
}

// NewValidateAppDefinitionHandler creates a new handler.
func NewValidateAppDefinitionHandler() *ValidateAppDefinitionHandler {
	return &ValidateAppDefinitionHandler{
		skill: skills.NewValidateAppDefinitionSkill(),
	}
}

func (h *ValidateAppDefinitionHandler) Name() string {
	return "validate_app_definition"
}

func (h *ValidateAppDefinitionHandler) Description() string {
	return "Validate a generated Radius application definition (app.bicep) for correctness"
}

func (h *ValidateAppDefinitionHandler) InputSchema() json.RawMessage {
	return json.RawMessage(`{
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
	}`)
}

func (h *ValidateAppDefinitionHandler) Handle(ctx context.Context, input json.RawMessage) (interface{}, error) {
	var params struct {
		BicepContent    string                     `json:"bicepContent"`
		FilePath        string                     `json:"filePath"`
		DiscoveryResult *discovery.DiscoveryResult `json:"discoveryResult"`
		StrictMode      bool                       `json:"strictMode"`
	}

	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if params.BicepContent == "" && params.FilePath == "" {
		return nil, fmt.Errorf("either bicepContent or filePath is required")
	}

	skillInput := &skills.ValidateAppDefinitionInput{
		BicepContent:    params.BicepContent,
		FilePath:        params.FilePath,
		DiscoveryResult: params.DiscoveryResult,
		StrictMode:      params.StrictMode,
	}

	result, err := h.skill.Execute(skillInput)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// DiscoverRecipesHandler handles the discover_recipes tool.
type DiscoverRecipesHandler struct {
	skill *skills.DiscoverRecipesSkill
}

// NewDiscoverRecipesHandler creates a new handler.
func NewDiscoverRecipesHandler() *DiscoverRecipesHandler {
	return &DiscoverRecipesHandler{
		skill: skills.NewDiscoverRecipesSkill(),
	}
}

func (h *DiscoverRecipesHandler) Name() string {
	return "discover_recipes"
}

func (h *DiscoverRecipesHandler) Description() string {
	return "Match discovered infrastructure dependencies to available Radius recipes from various sources"
}

func (h *DiscoverRecipesHandler) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"resourceTypeMappings": {
				"type": "array",
				"description": "Resource type mappings from generate_resource_types",
				"items": {
					"type": "object",
					"properties": {
						"dependencyId": {"type": "string"},
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
				"description": "Filter recipes by cloud provider (aws, azure, gcp)",
				"enum": ["aws", "azure", "gcp", ""]
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
	}`)
}

func (h *DiscoverRecipesHandler) Handle(ctx context.Context, input json.RawMessage) (interface{}, error) {
	var params struct {
		ResourceTypeMappings []discovery.ResourceTypeMapping `json:"resourceTypeMappings"`
		CloudProvider        string                          `json:"cloudProvider"`
		MinConfidence        float64                         `json:"minConfidence"`
		MaxMatchesPerType    int                             `json:"maxMatchesPerType"`
		PreferredSources     []string                        `json:"preferredSources"`
	}

	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	if len(params.ResourceTypeMappings) == 0 {
		return nil, fmt.Errorf("resourceTypeMappings is required")
	}

	result, err := h.skill.DiscoverRecipes(ctx, skills.DiscoverRecipesInput{
		ResourceTypeMappings: params.ResourceTypeMappings,
		CloudProvider:        params.CloudProvider,
		MinConfidence:        params.MinConfidence,
		MaxMatchesPerType:    params.MaxMatchesPerType,
		PreferredSources:     params.PreferredSources,
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}
