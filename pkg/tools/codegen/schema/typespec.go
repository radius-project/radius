// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	"encoding/json"
)

// TypeSpec contains the defitinitions of a type.
type TypeSpec struct {
	OneOf      []*TypeRef
	Properties map[string]*PropertySpec

	AdditionalProperties map[string]interface{}
}

// InlineAllRefs makes all the type inlined, when we merge all the
// files into one.
func (t *TypeSpec) InlineAllRefs() {
	for i, r := range t.OneOf {
		t.OneOf[i] = r.Inlined()
	}
	for _, p := range t.Properties {
		p.InlineAllRefs()
	}
	inlineAllRef(t.AdditionalProperties)
}

// MarshalJSON implements custom JSON serialization logic.
func (t *TypeSpec) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})
	if len(t.OneOf) > 0 {
		m["oneOf"] = t.OneOf
	}
	if len(t.Properties) > 0 {
		m["properties"] = t.Properties
	}
	for a, p := range t.AdditionalProperties {
		m[a] = p
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements custom JSON deserialization logic.
func (t *TypeSpec) UnmarshalJSON(b []byte) error {
	// We choose the suboptimal way of deserializing twice to support
	// additionalProperties. This is for simplicity
	//
	// This isn't ideal, so don't use this code on a performance sensitive path.
	inner := struct {
		OneOf      []*TypeRef
		Properties map[string]*PropertySpec
	}{}
	if err := json.Unmarshal(b, &inner); err != nil {
		return err
	}
	additionalProperties := make(map[string]interface{})
	if err := json.Unmarshal(b, &additionalProperties); err != nil {
		return err
	}
	for _, field := range []string{"oneOf", "properties"} {
		delete(additionalProperties, field)
	}
	*t = TypeSpec{
		OneOf:                inner.OneOf,
		Properties:           inner.Properties,
		AdditionalProperties: additionalProperties,
	}
	return nil
}
