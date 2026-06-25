package converter

import (
	"slices"

	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
)

// baseResourceProperties are the common "Radius-aware" properties that every
// resource type schema inherits: application, environment, connections, and
// codeReference.
//
// This is a hand-maintained copy of the canonical source of truth,
// pkg/schema/baseresource/base.yaml. bicep-tools models schemas with its own
// strongly typed manifest.Schema (rather than the map[string]any the schema
// package merges), so the base set is duplicated here as a typed literal. The
// sync test TestApplyBaseResource_PropertiesMatchCanonicalYAML fails CI if this
// literal drifts from base.yaml.
var baseResourceProperties = map[string]manifest.Schema{
	"application": {
		Type:        "string",
		Description: ptr("Resource ID of the Radius.Core/applications this resource belongs to."),
	},
	"environment": {
		Type:        "string",
		Description: ptr("Resource ID of the Radius.Core/environments this resource deploys into."),
	},
	"connections": {
		Type:        "object",
		Description: ptr("Map of connection name to connection data."),
		AdditionalProperties: &manifest.Schema{
			Type: "object",
			Properties: map[string]manifest.Schema{
				"source": {
					Type:        "string",
					Description: ptr("Resource ID of the source resource for this connection."),
				},
				"disableDefaultEnvVars": {
					Type:        "boolean",
					Description: ptr("Disables the automatic injection of environment variables from the connected resource's properties."),
				},
			},
			Required: []string{"source"},
		},
	},
	"codeReference": {
		Type:        "string",
		Description: ptr("Optional URI to the source code of this resource type. ex: https://github.com/radius-project/radius/blob/4fab87e8127adf1db6f43b7029d5235fbe82c5c9/cmd/controller/main.go#L27"),
	},
}

// baseResourceRequired lists the common properties that are mandatory on every
// resource type. It mirrors the "required" section of base.yaml.
var baseResourceRequired = []string{"environment"}

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

// ptr returns a pointer to v.
func ptr[T any](v T) *T {
	return &v
}
