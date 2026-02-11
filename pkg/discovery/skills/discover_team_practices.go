// Package skills provides composable skill implementations for discovery operations.
package skills

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/radius-project/radius/pkg/discovery/practices"
)

// DiscoverTeamPracticesInput defines the input for the discover_team_practices skill.
type DiscoverTeamPracticesInput struct {
	// ProjectPath is the root path of the project to analyze.
	ProjectPath string `json:"project_path"`

	// Environment is the target environment (e.g., "dev", "staging", "prod").
	// Used to select environment-specific practices.
	Environment string `json:"environment,omitempty"`

	// IncludeIaC indicates whether to analyze existing IaC files for practices.
	IncludeIaC bool `json:"include_iac,omitempty"`

	// ConfigPath is an optional path to a team practices config file.
	// Defaults to .radius/team-practices.yaml in the project.
	ConfigPath string `json:"config_path,omitempty"`
}

// DiscoverTeamPracticesOutput defines the output from the discover_team_practices skill.
type DiscoverTeamPracticesOutput struct {
	// Practices contains the discovered team practices.
	Practices *practices.TeamPractices `json:"practices"`

	// Sources lists where practices were discovered from.
	Sources []practices.PracticeSource `json:"sources"`

	// Environment is the resolved environment name.
	Environment string `json:"environment,omitempty"`

	// ConfigPath is the path to the config file if one was used.
	ConfigPath string `json:"config_path,omitempty"`
}

// DiscoverTeamPracticesSkill discovers team infrastructure practices from
// configuration files and existing IaC.
type DiscoverTeamPracticesSkill struct{}

// NewDiscoverTeamPracticesSkill creates a new DiscoverTeamPracticesSkill.
func NewDiscoverTeamPracticesSkill() *DiscoverTeamPracticesSkill {
	return &DiscoverTeamPracticesSkill{}
}

// Name returns the skill name.
func (s *DiscoverTeamPracticesSkill) Name() string {
	return "discover_team_practices"
}

// Description returns the skill description.
func (s *DiscoverTeamPracticesSkill) Description() string {
	return "Discover team infrastructure practices from configuration and existing IaC files"
}

// Execute runs the discover_team_practices skill.
func (s *DiscoverTeamPracticesSkill) Execute(ctx context.Context, input DiscoverTeamPracticesInput) (*DiscoverTeamPracticesOutput, error) {
	if input.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	output := &DiscoverTeamPracticesOutput{
		Practices:   &practices.TeamPractices{Tags: make(map[string]string)},
		Environment: input.Environment,
	}

	// Load from config file if specified or exists
	configPath := input.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(input.ProjectPath, ".radius", "team-practices.yaml")
	}

	configPractices, err := practices.LoadConfigFromFile(configPath)
	if err == nil && configPractices != nil {
		output.Practices.Merge(&configPractices.Practices)
		output.ConfigPath = configPath
		output.Sources = append(output.Sources, practices.PracticeSource{
			Type:       practices.SourceConfig,
			FilePath:   configPath,
			Confidence: 1.0, // Config files are authoritative
		})
	}

	// Analyze IaC files if requested
	if input.IncludeIaC {
		// Try Terraform files
		tfParser := practices.NewTerraformParser(input.ProjectPath)
		tfPractices, err := tfParser.Parse()
		if err == nil && tfPractices != nil {
			output.Practices.Merge(tfPractices)
			output.Sources = append(output.Sources, tfPractices.Sources...)
		}

		// Try Bicep files
		bicepParser := practices.NewBicepParser(input.ProjectPath)
		bicepPractices, err := bicepParser.Parse()
		if err == nil && bicepPractices != nil {
			output.Practices.Merge(bicepPractices)
			output.Sources = append(output.Sources, bicepPractices.Sources...)
		}
	}

	// Apply environment-specific overrides if specified
	if input.Environment != "" && output.Practices != nil {
		envTier := output.Practices.GetTierForEnvironment(input.Environment)
		if envTier != "" {
			output.Practices.Sizing.DefaultTier = envTier
		}
	}

	// Deduplicate sources
	output.Sources = deduplicateSources(output.Sources)

	// Calculate overall confidence
	if len(output.Sources) > 0 {
		var totalConfidence float64
		for _, src := range output.Sources {
			totalConfidence += src.Confidence
		}
		output.Practices.Confidence = totalConfidence / float64(len(output.Sources))
	}

	return output, nil
}

// deduplicateSources removes duplicate sources based on file path.
func deduplicateSources(sources []practices.PracticeSource) []practices.PracticeSource {
	seen := make(map[string]bool)
	result := make([]practices.PracticeSource, 0, len(sources))

	for _, src := range sources {
		key := string(src.Type) + ":" + src.FilePath
		if !seen[key] {
			seen[key] = true
			result = append(result, src)
		}
	}

	return result
}

// ValidateInput validates the input parameters.
func (s *DiscoverTeamPracticesSkill) ValidateInput(input DiscoverTeamPracticesInput) error {
	if input.ProjectPath == "" {
		return fmt.Errorf("project_path is required")
	}
	return nil
}

// GetSchema returns the JSON schema for the skill input.
func (s *DiscoverTeamPracticesSkill) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"project_path": map[string]interface{}{
				"type":        "string",
				"description": "Root path of the project to analyze",
			},
			"environment": map[string]interface{}{
				"type":        "string",
				"description": "Target environment (dev, staging, prod)",
			},
			"include_iac": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to analyze existing IaC files for practices",
				"default":     true,
			},
			"config_path": map[string]interface{}{
				"type":        "string",
				"description": "Optional path to team practices config file",
			},
		},
		"required": []string{"project_path"},
	}
}
