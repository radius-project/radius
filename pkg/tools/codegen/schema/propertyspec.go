// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import "encoding/json"

// PropertySpec contains the specifications of a type's property.
type PropertySpec struct {
	Enum                 []interface{}
	Description          string
	Type                 string
	AdditionalProperties map[string]interface{}
}

// InlineAllRefs makes all the type inlined, when we merge all the
// files into one.
func (p *PropertySpec) InlineAllRefs() {
	inlineAllRef(p.AdditionalProperties)
}

// MarshalJSON implements custom JSON serialization logic.
func (p *PropertySpec) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})

	if len(p.Enum) > 0 {
		m["enum"] = p.Enum
	}
	if p.Description != "" {
		m["description"] = p.Description
	}
	if p.Type != "" {
		m["type"] = p.Type
	}
	for k, v := range p.AdditionalProperties {
		m[k] = v
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements custom JSON deserialization logic.
func (p *PropertySpec) UnmarshalJSON(b []byte) error {
	// We choose the suboptimal way of deserializing twice to support
	// additionalProperties. This is for simplicity
	//
	// This isn't ideal, so don't use this code on a performance sensitive path.
	inner := struct {
		Enum        []interface{} `json:"enum"`
		Description string        `json:"description"`
		Type        string        `json:"type"`
	}{}
	if err := json.Unmarshal(b, &inner); err != nil {
		return err
	}
	additionalProperties := make(map[string]interface{})
	if err := json.Unmarshal(b, &additionalProperties); err != nil {
		return err
	}
	delete(additionalProperties, "enum")
	delete(additionalProperties, "description")
	delete(additionalProperties, "type")
	if len(additionalProperties) == 0 {
		additionalProperties = nil
	}
	*p = PropertySpec{
		Enum:                 inner.Enum,
		Description:          inner.Description,
		Type:                 inner.Type,
		AdditionalProperties: additionalProperties,
	}
	return nil
}
