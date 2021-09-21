// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	_ "embed"
	"fmt"

	"github.com/Azure/radius/pkg/radrp/schemav3"
)

//go:embed boilerplate.json
var boilerplate string

// AutorestConverter converts a JSON schema into a autorest-compatible
// OpenAPI doc.
type AutorestConverter interface {
	Convert(map[string]Schema) (*Schema, error)
}

// NewAutorestConverter creates a AutorestConverter.
func NewAutorestConverter() *converter {
	resourceTypes := []resourceInfo{}
	for qualifiedName, resourcePath := range schemav3.ResourceManifest.Resources {
		resourceTypes = append(resourceTypes, newResourceInfo(qualifiedName, resourcePath))
	}
	return &converter{
		resources: resourceTypes,
	}
}

type converter struct {
	resources []resourceInfo
}

func (c *converter) handlePolymorphism(s *Schema) (*Schema, error) {
	// Fill in the autorest specific polymorphic type annotations.
	for parent, spec := range s.Definitions {
		if len(spec.OneOf) == 0 {
			continue
		}
		// We are dealing with polymorphic type.
		//
		// For now, we only support "kind"
		spec.AdditionalProperties["discriminator"] = "kind"

		// Mark the field as required. Instead of using
		// []string{"kind"} here, we use []interface{}{"kind"} to make
		// it easier to unmarshal an output file and test for exact
		// match.  This usage does not affect the produced JSON files.
		spec.AdditionalProperties["required"] = []interface{}{"kind"}
		parentRef := TypeRef("#/definitions/" + parent)
		for _, childref := range spec.OneOf {
			childname := childref.Name()
			child := s.Definitions[childname]
			if child == nil {
				return nil, fmt.Errorf("missing definition of child type %s", childname)
			}
			// Add 'allOf' pointing to parent ref. interface{}-based
			// types were used to make it easier to unmarshal an
			// output file and test for exact match. This usage does not affect
			// the produced JSON files.
			child.AdditionalProperties["allOf"] = []interface{}{map[string]interface{}{
				"$ref": string(parentRef),
			}}
			kindProp, ok := child.Properties["kind"]
			if !ok || len(kindProp.Enum) != 1 {
				return nil, fmt.Errorf("type %q does not have a kind of single enum value", childname)
			}
			child.AdditionalProperties["x-ms-discriminator-value"] = kindProp.Enum[0]
			// Remove the child kind's property, which autorest doesn't like
			delete(child.Properties, "kind")
		}

		// Remove oneOf, which autorest does not like.
		spec.OneOf = nil
	}
	return s, nil
}

// loadBoilerPlate loads the boilerplate as a Schema.
func (c *converter) loadBoilerPlate() (*Schema, error) {
	return LoadBytes([]byte(boilerplate))
}

func (c *converter) Convert(schemas map[string]Schema) (*Schema, error) {
	// Merge all of them.
	s, err := c.Merge(schemas)
	if err != nil {
		return nil, err
	}
	s, err = c.handlePolymorphism(s)
	if err != nil {
		return nil, err
	}

	// Merge the boilerplate schema.
	boilerplate, err := c.loadBoilerPlate()
	if err != nil {
		return nil, fmt.Errorf("failed loading boilerplate schema: %w", err)
	}
	boilerplates := []*Schema{boilerplate}
	for _, r := range c.resources {
		pathSchema, err := LoadResourceBoilerplateSchemaForType(r)
		if err != nil {
			return nil, fmt.Errorf("failed loading path schema for %s: %w", r, err)
		}
		boilerplates = append(boilerplates, pathSchema)
	}

	for _, boilerplate := range boilerplates {
		s, err = s.Merge(boilerplate)
		if err != nil {
			return nil, fmt.Errorf("failed merging boilerplate schema: %w", err)
		}
	}

	// Remove disallowed properties
	for _, disallowed := range []string{"$schema"} {
		delete(s.AdditionalProperties, disallowed)
	}
	return s, err
}

func (c *converter) Merge(schemas map[string]Schema) (*Schema, error) {
	out := NewSchema()

	for name, schema := range schemas {
		merged, err := out.Merge(&schema)

		if err != nil {
			return nil, fmt.Errorf("fail to merge schema from file %s: %w", name, err)
		}
		*out = *merged
	}

	out.InlineAllRefs()
	return out, nil
}
