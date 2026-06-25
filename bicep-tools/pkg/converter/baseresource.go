package converter

import (
	"fmt"
	"slices"

	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
	"github.com/radius-project/radius/pkg/schema/baseresource"

	"go.yaml.in/yaml/v3"
)

// baseResourceProperties and baseResourceRequired are the common "Radius-aware"
// properties that every resource type schema inherits (application, environment,
// connections, and codeReference) plus the subset that is mandatory.
//
// They are decoded once, at package initialization, from the canonical
// source of truth — pkg/schema/baseresource/base.yaml — into bicep-tools' own
// strongly typed manifest.Schema model. Decoding the same bytes the schema
// package embeds means the base set is never duplicated and cannot drift.
var baseResourceProperties, baseResourceRequired = mustParseBaseResource()

// mustParseBaseResource decodes the embedded canonical base manifest into the
// bicep-tools manifest.Schema model. The manifest is a build-time constant, so a
// parse failure indicates a programming error and panics at startup.
func mustParseBaseResource() (map[string]manifest.Schema, []string) {
	var doc struct {
		Properties map[string]manifest.Schema `yaml:"properties"`
		Required   []string                   `yaml:"required"`
	}
	if err := yaml.Unmarshal(baseresource.RawManifest(), &doc); err != nil {
		panic(fmt.Sprintf("bicep-tools: failed to parse base resource manifest: %v", err))
	}
	if len(doc.Properties) == 0 {
		panic("bicep-tools: base resource manifest declares no properties")
	}
	return doc.Properties, doc.Required
}

// applyBaseResource merges the common base properties into the given schema in
// place, mirroring (*baseresource.BaseManifest).Apply in the schema package: for
// each base property absent from schema.Properties it copies the base shape in
// (per-type-wins — an author's own declaration is never overwritten), then
// unions baseResourceRequired into schema.Required. It is idempotent and a nil
// schema is a no-op.
func applyBaseResource(schema *manifest.Schema) {
	if schema == nil {
		return
	}

	if schema.Properties == nil {
		schema.Properties = map[string]manifest.Schema{}
	}

	for name, prop := range baseResourceProperties {
		if _, exists := schema.Properties[name]; exists {
			continue
		}
		schema.Properties[name] = prop
	}

	for _, required := range baseResourceRequired {
		if !slices.Contains(schema.Required, required) {
			schema.Required = append(schema.Required, required)
		}
	}
}
