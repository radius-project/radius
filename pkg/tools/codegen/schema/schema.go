// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
)

// Schema represents a OpenAPI schema spec.
type Schema struct {
	Definitions          map[string]*TypeSpec
	AdditionalProperties map[string]interface{}
}

// NewSchema creates an empty schema.
func NewSchema() *Schema {
	return &Schema{
		Definitions:          make(map[string]*TypeSpec),
		AdditionalProperties: make(map[string]interface{}),
	}
}

// InlineAllRefs makes all the type inlined, when we merge all the
// files into one.
func (s *Schema) InlineAllRefs() {
	for _, d := range s.Definitions {
		d.InlineAllRefs()
	}
	inlineAllRef(s.AdditionalProperties)
}

func inlineAllRef(m map[string]interface{}) {
	r, hasRef := m["$ref"]
	if hasRef {
		if refName, ok := r.(string); ok {
			m["$ref"] = TypeRef(refName).InlinedName()
		}
	}
	for _, v := range m {
		if obj, ok := v.(map[string]interface{}); ok {
			inlineAllRef(obj)
		}
		if arr, ok := v.([]interface{}); ok {
			inlineAllRefArr(arr)
		}
	}
}

func inlineAllRefArr(arr []interface{}) {
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			inlineAllRef(obj)
		}
		if arr, ok := item.([]interface{}); ok {
			inlineAllRefArr(arr)
		}
	}
}

// MarshalJSON implements custom JSON serialization logic.
func (s *Schema) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})

	if len(s.Definitions) > 0 {
		m["definitions"] = &s.Definitions
	}
	for k, v := range s.AdditionalProperties {
		m[k] = v
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements custom JSON deserialization logic.
func (s *Schema) UnmarshalJSON(b []byte) error {
	// We choose the suboptimal way of deserializing twice to support
	// additionalProperties. This is for simplicity
	//
	// This isn't ideal, so don't use this code on a performance sensitive path.
	inner := struct {
		Definitions map[string]*TypeSpec `json:",omitempty"`
	}{}
	if err := json.Unmarshal(b, &inner); err != nil {
		return fmt.Errorf("failed parsing Schema: %w", err)
	}
	additionalProperties := make(map[string]interface{})
	if err := json.Unmarshal(b, &additionalProperties); err != nil {
		return err
	}
	delete(additionalProperties, "definitions")
	*s = Schema{
		Definitions:          inner.Definitions,
		AdditionalProperties: additionalProperties,
	}
	return nil
}

// Merge merges two schema into one.
func (s *Schema) Merge(o *Schema) (*Schema, error) {
	out := *s
	for d, t := range o.Definitions {
		_, existed := s.Definitions[d]
		if existed {
			return nil, fmt.Errorf("duplicate definitions %s", d)
		}
		out.Definitions[d] = t
	}
	for k, src := range o.AdditionalProperties {
		dest, existed := s.AdditionalProperties[k]
		if !existed {
			out.AdditionalProperties[k] = src
			continue
		}
		srcMap, srcIsMap := src.(map[string]interface{})
		destMap, destIsMap := dest.(map[string]interface{})

		// For map, we merge, but for non-maps we only accept exact matches.
		if !srcIsMap || !destIsMap {
			if !reflect.DeepEqual(src, dest) {
				return nil, fmt.Errorf("duplicate non-map property %q at the top level", k)
			}
			continue
		}
		for kk, vv := range srcMap {
			destMap[kk] = vv
		}
	}
	return &out, nil
}

func LoadBytes(b []byte) (*Schema, error) {
	s := Schema{}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Load loads a Schema from a given file.
func Load(filename string) (*Schema, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return LoadBytes(b)
}
