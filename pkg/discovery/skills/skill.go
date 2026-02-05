// Package skills provides the composable skill framework for discovery operations.
// Skills are self-contained units that perform specific discovery tasks and can
// be orchestrated by the discovery engine or exposed via MCP.
package skills

import (
	"context"
	"fmt"
)

// Skill is the interface for a discovery skill.
// Skills are composable units that perform specific discovery tasks.
type Skill interface {
	// Name returns the skill's unique identifier.
	Name() string

	// Description returns a human-readable description of the skill.
	Description() string

	// InputSchema returns JSON Schema for the skill's input parameters.
	InputSchema() map[string]interface{}

	// Execute runs the skill with the given input.
	Execute(ctx context.Context, input SkillInput) (SkillOutput, error)
}

// SkillInput contains the input parameters for a skill invocation.
type SkillInput struct {
	// ProjectPath is the path to the project being analyzed.
	ProjectPath string `json:"projectPath"`

	// Parameters contains skill-specific parameters.
	Parameters map[string]interface{} `json:"parameters,omitempty"`

	// Context provides results from previously executed skills.
	Context *SkillContext `json:"context,omitempty"`
}

// SkillContext holds accumulated results from skill executions.
type SkillContext struct {
	// DetectedServices from discover_services skill.
	Services []interface{} `json:"services,omitempty"`

	// DetectedDependencies from discover_dependencies skill.
	Dependencies []interface{} `json:"dependencies,omitempty"`

	// TeamPractices from discover_team_practices skill.
	Practices map[string]interface{} `json:"practices,omitempty"`

	// ResourceTypes from generate_resource_types skill.
	ResourceTypes []interface{} `json:"resourceTypes,omitempty"`

	// RecipeMatches from discover_recipes skill.
	Recipes []interface{} `json:"recipes,omitempty"`

	// Custom data from other skills.
	Custom map[string]interface{} `json:"custom,omitempty"`
}

// NewSkillContext creates an empty skill context.
func NewSkillContext() *SkillContext {
	return &SkillContext{
		Custom: make(map[string]interface{}),
	}
}

// SkillOutput contains the result of a skill execution.
type SkillOutput struct {
	// Success indicates whether the skill completed successfully.
	Success bool `json:"success"`

	// Result contains the skill-specific output data.
	Result interface{} `json:"result,omitempty"`

	// Warnings encountered during execution.
	Warnings []string `json:"warnings,omitempty"`

	// Error message if Success is false.
	Error string `json:"error,omitempty"`
}

// NewSuccessOutput creates a successful skill output.
func NewSuccessOutput(result interface{}) SkillOutput {
	return SkillOutput{
		Success: true,
		Result:  result,
	}
}

// NewErrorOutput creates a failed skill output.
func NewErrorOutput(err error) SkillOutput {
	return SkillOutput{
		Success: false,
		Error:   err.Error(),
	}
}

// SkillRegistry manages available skills.
type SkillRegistry struct {
	skills map[string]Skill
}

// NewSkillRegistry creates a new skill registry.
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills: make(map[string]Skill),
	}
}

// Register adds a skill to the registry.
func (r *SkillRegistry) Register(skill Skill) error {
	name := skill.Name()
	if _, exists := r.skills[name]; exists {
		return fmt.Errorf("skill %q already registered", name)
	}
	r.skills[name] = skill
	return nil
}

// Get returns a skill by name.
func (r *SkillRegistry) Get(name string) (Skill, bool) {
	skill, ok := r.skills[name]
	return skill, ok
}

// All returns all registered skills.
func (r *SkillRegistry) All() []Skill {
	result := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	return result
}

// Names returns the names of all registered skills.
func (r *SkillRegistry) Names() []string {
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	return names
}

// DefaultSkillRegistry is the global skill registry.
var DefaultSkillRegistry = NewSkillRegistry()

// Register adds a skill to the default registry.
func Register(skill Skill) error {
	return DefaultSkillRegistry.Register(skill)
}
