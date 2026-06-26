package converter

import (
	"fmt"
	"slices"

	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
	"github.com/radius-project/radius/pkg/schema/baseresource"

	"go.yaml.in/yaml/v3"
)

// baseResource holds the common "Radius-aware" properties (application,
// environment, connections, codeReference) that every resource type schema
// inherits, plus the subset that is mandatory.
//
// It is decoded from the canonical source of truth —
// pkg/schema/baseresource/base.yaml — into bicep-tools' own strongly typed
// manifest.Schema model.
type baseResource struct {
	properties map[string]manifest.Schema
	required   []string
}

// loadBaseResource decodes the embedded canonical base manifest into a
// baseResource. The manifest is a build-time constant, so a decode failure
// indicates a programming error in the embedded YAML.
func loadBaseResource() (*baseResource, error) {
	var base struct {
		Properties map[string]manifest.Schema `yaml:"properties"`
		Required   []string                   `yaml:"required"`
	}
	if err := yaml.Unmarshal(baseresource.RawManifest(), &base); err != nil {
		return nil, fmt.Errorf("bicep-tools: failed to parse base resource manifest: %w", err)
	}
	if len(base.Properties) == 0 {
		return nil, fmt.Errorf("bicep-tools: base resource manifest declares no properties")
	}
	return &baseResource{properties: base.Properties, required: base.Required}, nil
}

// apply merges the common base properties into the given schema in place.
func (b *baseResource) apply(schema *manifest.Schema) {
	if schema == nil {
		return
	}
	if schema.Properties == nil {
		schema.Properties = map[string]manifest.Schema{}
	}

	for name, prop := range b.properties {
		if _, exists := schema.Properties[name]; exists {
			continue
		}
		schema.Properties[name] = prop
	}

	for _, required := range b.required {
		if !slices.Contains(schema.Required, required) {
			schema.Required = append(schema.Required, required)
		}
	}
}
