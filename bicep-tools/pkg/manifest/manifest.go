package manifest

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ResourceProvider represents a Radius resource provider manifest
// Equivalent to TypeScript interface ResourceProvider
type ResourceProvider struct {
	Namespace string                  `yaml:"namespace" json:"namespace"`
	Types     map[string]ResourceType `yaml:"types" json:"types"`
}

// ResourceType represents a resource type definition within a provider
// Equivalent to TypeScript interface ResourceType
type ResourceType struct {
	DefaultAPIVersion *string               `yaml:"defaultApiVersion,omitempty" json:"defaultApiVersion,omitempty"`
	APIVersions       map[string]APIVersion `yaml:"apiVersions" json:"apiVersions"`
}

// APIVersion represents a specific API version of a resource type
// Equivalent to TypeScript interface APIVersion
type APIVersion struct {
	Schema       Schema   `yaml:"schema" json:"schema"`
	Capabilities []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
}

// Schema represents the schema definition for a resource type
// Equivalent to TypeScript interface Schema
type Schema struct {
	Type                 string            `yaml:"type" json:"type"`
	Description          *string           `yaml:"description,omitempty" json:"description,omitempty"`
	Properties           map[string]Schema `yaml:"properties,omitempty" json:"properties,omitempty"`
	AdditionalProperties *Schema           `yaml:"additionalProperties,omitempty" json:"additionalProperties,omitempty"`
	Required             []string          `yaml:"required,omitempty" json:"required,omitempty"`
	ReadOnly             *bool             `yaml:"readOnly,omitempty" json:"readOnly,omitempty"`
}

// ParseManifest parses a YAML manifest string into a ResourceProvider struct
// Equivalent to TypeScript function parseManifest
func ParseManifest(input string) (*ResourceProvider, error) {
	var provider ResourceProvider
	if err := yaml.Unmarshal([]byte(input), &provider); err != nil {
		return nil, fmt.Errorf("failed to parse YAML manifest: %w", err)
	}

	// Validate required fields
	if provider.Namespace == "" {
		return nil, fmt.Errorf("manifest name is required")
	}

	if provider.Types == nil {
		return nil, fmt.Errorf("manifest types are required")
	}

	return &provider, nil
}

// Validate performs basic validation on the resource provider manifest
func (rp *ResourceProvider) Validate() error {
	if rp.Namespace == "" {
		return fmt.Errorf("resource provider name cannot be empty")
	}

	if len(rp.Types) == 0 {
		return fmt.Errorf("resource provider must have at least one type")
	}

	for typeName, resourceType := range rp.Types {
		if err := resourceType.Validate(typeName); err != nil {
			return fmt.Errorf("validation failed for type '%s': %w", typeName, err)
		}
	}

	return nil
}

// Validate performs validation on a resource type
func (rt *ResourceType) Validate(typeName string) error {
	if len(rt.APIVersions) == 0 {
		return fmt.Errorf("resource type '%s' must have at least one API version", typeName)
	}

	for apiVersion, apiVersionDef := range rt.APIVersions {
		if err := apiVersionDef.Validate(typeName, apiVersion); err != nil {
			return fmt.Errorf("validation failed for API version '%s': %w", apiVersion, err)
		}
	}

	return nil
}

// Validate performs validation on an API version
func (av *APIVersion) Validate(typeName, apiVersion string) error {
	return av.Schema.Validate(fmt.Sprintf("%s@%s", typeName, apiVersion))
}

// Validate performs validation on a schema
func (s *Schema) Validate(context string) error {
	// If type is empty, default to "object" (matches TypeScript behavior for empty schemas)
	if s.Type == "" {
		s.Type = "object"
	}

	validTypes := map[string]bool{
		"string":  true,
		"object":  true,
		"integer": true,
		"boolean": true,
		"any":     true,
	}

	if !validTypes[s.Type] {
		return fmt.Errorf("invalid schema type '%s' in %s", s.Type, context)
	}

	// Validate nested properties if this is an object type
	if s.Type == "object" && s.Properties != nil {
		for propName, propSchema := range s.Properties {
			if err := propSchema.Validate(fmt.Sprintf("%s.%s", context, propName)); err != nil {
				return err
			}
		}
	}

	return nil
}
