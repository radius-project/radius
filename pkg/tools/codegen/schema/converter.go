// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import "fmt"

// AutorestConverter converts a JSON schema into a autorest-compatible
// OpenAPI doc.
type AutorestConverter interface {
	Convert(map[string]Schema) (*Schema, error)
}

// NewAutorestConverter creates a AutorestConverter.
func NewAutorestConverter() AutorestConverter {
	return &converter{}
}

type converter struct{}

func (c *converter) Convert(schemas map[string]Schema) (*Schema, error) {
	// Merge all of them.
	s, err := c.Merge(schemas)
	if err != nil {
		return nil, err
	}

	// Fill in the autorest specific polymorphic type annotations.
	for parent, spec := range s.Definitions {
		if len(spec.OneOf) == 0 {
			continue
		}
		// We are dealing with polymorphic type.
		//
		// For now, we only support "kind"
		spec.AdditionalProperties["discriminator"] = "kind"
		parentRef := TypeRef("#/definitions/" + parent)
		for _, childref := range spec.OneOf {
			childname := childref.Name()
			child := s.Definitions[childname]
			if child == nil {
				return nil, fmt.Errorf("missing definition of child type %s", childname)
			}
			child.AdditionalProperties["allOf"] = []*TypeRef{
				&parentRef,
			}
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
