package skills

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/practices"
	"github.com/radius-project/radius/pkg/discovery/resourcetypes"
)

// GenerateResourceTypesSkill maps detected dependencies to Radius Resource Types.
type GenerateResourceTypesSkill struct {
	catalog   *resourcetypes.Catalog
	practices *practices.TeamPractices
}

// NewGenerateResourceTypesSkill creates the generate_resource_types skill.
func NewGenerateResourceTypesSkill(catalog *resourcetypes.Catalog) *GenerateResourceTypesSkill {
	return &GenerateResourceTypesSkill{catalog: catalog}
}

// Name returns the skill identifier.
func (s *GenerateResourceTypesSkill) Name() string {
	return "generate_resource_types"
}

// Description returns a human-readable description.
func (s *GenerateResourceTypesSkill) Description() string {
	return "Maps detected infrastructure dependencies to valid Radius Resource Type definitions using a pre-defined catalog."
}

// InputSchema returns the JSON Schema for input parameters.
func (s *GenerateResourceTypesSkill) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"dependencies": map[string]interface{}{
				"type":        "array",
				"description": "Array of detected dependencies to map",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":   map[string]interface{}{"type": "string"},
						"type": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
		"required": []string{"dependencies"},
	}
}

// Execute runs the skill.
func (s *GenerateResourceTypesSkill) Execute(ctx context.Context, input SkillInput) (SkillOutput, error) {
	// Get dependencies from context or parameters
	var dependencies []discovery.DetectedDependency

	if input.Context != nil && len(input.Context.Dependencies) > 0 {
		// Convert from interface to DetectedDependency
		for _, depInterface := range input.Context.Dependencies {
			if depMap, ok := depInterface.(map[string]interface{}); ok {
				dep := discovery.DetectedDependency{
					ID:         getString(depMap, "id"),
					Type:       discovery.DependencyType(getString(depMap, "type")),
					Name:       getString(depMap, "name"),
					Library:    getString(depMap, "library"),
					Confidence: getFloat(depMap, "confidence"),
				}
				dependencies = append(dependencies, dep)
			}
		}
	}

	// Also check parameters
	if depsParam, ok := input.Parameters["dependencies"].([]interface{}); ok {
		for _, depInterface := range depsParam {
			if depMap, ok := depInterface.(map[string]interface{}); ok {
				dep := discovery.DetectedDependency{
					ID:         getString(depMap, "id"),
					Type:       discovery.DependencyType(getString(depMap, "type")),
					Name:       getString(depMap, "name"),
					Library:    getString(depMap, "library"),
					Confidence: getFloat(depMap, "confidence"),
				}
				dependencies = append(dependencies, dep)
			}
		}
	}

	if len(dependencies) == 0 {
		return NewSuccessOutput(map[string]interface{}{
			"resourceTypes": []discovery.ResourceTypeMapping{},
			"count":         0,
		}), nil
	}

	// Map dependencies to Resource Types
	var mappings []discovery.ResourceTypeMapping
	var warnings []string

	for _, dep := range dependencies {
		entry, found := s.catalog.Lookup(dep.Type)
		if !found {
			// Try dynamic lookup from resource-types-contrib
			contribType, err := resourcetypes.LookupFromContrib(ctx, dep.Type)
			if err == nil && contribType != nil {
				// Found a match in resource-types-contrib
				entry = contribType.ToResourceTypeEntry(dep.Type)
				found = true
			}
		}

		if !found {
			warnings = append(warnings, fmt.Sprintf("No Resource Type found for dependency type: %s", dep.Type))
			continue
		}

		mapping := resourcetypes.Match(dep, entry)

		// Apply team practices if available
		s.ApplyPractices(&mapping)

		mappings = append(mappings, mapping)
	}

	output := NewSuccessOutput(map[string]interface{}{
		"resourceTypes": mappings,
		"count":         len(mappings),
	})
	output.Warnings = warnings

	return output, nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0.0
}

func init() {
	if resourcetypes.DefaultCatalog != nil {
		skill := NewGenerateResourceTypesSkill(resourcetypes.DefaultCatalog)
		_ = Register(skill)
	}
}

// NewGenerateResourceTypesSkillWithDefaults creates a generate_resource_types skill with default catalog.
func NewGenerateResourceTypesSkillWithDefaults() *GenerateResourceTypesSkill {
	cat := resourcetypes.DefaultCatalog
	if cat == nil {
		cat = resourcetypes.NewCatalog()
	}

	return NewGenerateResourceTypesSkill(cat)
}

// WithPractices sets the team practices to apply to generated resource types.
func (s *GenerateResourceTypesSkill) WithPractices(p *practices.TeamPractices) *GenerateResourceTypesSkill {
	s.practices = p
	return s
}

// ApplyPractices applies team practices to a resource type mapping.
func (s *GenerateResourceTypesSkill) ApplyPractices(mapping *discovery.ResourceTypeMapping) {
	if s.practices == nil || mapping == nil {
		return
	}

	// Apply required tags to resource properties
	if len(s.practices.Tags) > 0 {
		if mapping.ResourceType.Properties == nil {
			mapping.ResourceType.Properties = make(map[string]interface{})
		}
		if mapping.ResourceType.Properties["tags"] == nil {
			mapping.ResourceType.Properties["tags"] = make(map[string]interface{})
		}
		if tags, ok := mapping.ResourceType.Properties["tags"].(map[string]interface{}); ok {
			for key, value := range s.practices.Tags {
				if _, exists := tags[key]; !exists {
					tags[key] = value
				}
			}
		}
	}

	// Apply security practices
	if s.practices.Security.EncryptionEnabled {
		if mapping.ResourceType.Properties == nil {
			mapping.ResourceType.Properties = make(map[string]interface{})
		}
		mapping.ResourceType.Properties["encryption"] = map[string]interface{}{
			"enabled": true,
		}
	}

	if s.practices.Security.TLSRequired {
		if mapping.ResourceType.Properties == nil {
			mapping.ResourceType.Properties = make(map[string]interface{})
		}
		mapping.ResourceType.Properties["tls"] = map[string]interface{}{
			"enabled":    true,
			"minVersion": s.practices.Security.MinTLSVersion,
		}
	}

	if s.practices.Security.PrivateNetworking {
		if mapping.ResourceType.Properties == nil {
			mapping.ResourceType.Properties = make(map[string]interface{})
		}
		mapping.ResourceType.Properties["networking"] = map[string]interface{}{
			"privateEndpointsEnabled": true,
		}
	}

	// Apply sizing defaults based on environment tier
	if s.practices.Sizing.DefaultTier != "" {
		if mapping.ResourceType.Properties == nil {
			mapping.ResourceType.Properties = make(map[string]interface{})
		}
		mapping.ResourceType.Properties["sku"] = s.practices.Sizing.DefaultTier
	}
}
