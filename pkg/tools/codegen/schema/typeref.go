// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TypeRef is a reference to a type.
type TypeRef string

// NewTypeRef creates a new type ref.
func NewTypeRef(s string) *TypeRef {
	new := TypeRef(s)
	return &new
}

// Name returns the name of the type reference.
func (t TypeRef) Name() string {
	tokens := strings.Split(string(t), "/")
	return tokens[len(tokens)-1]
}

// InlinedName strips out the file path from a type reference.
func (t TypeRef) InlinedName() string {
	tokens := strings.SplitN(string(t), "#", 2)
	return "#" + tokens[1]
}

// Inlined strips out the file path from a type reference.
func (t TypeRef) Inlined() *TypeRef {
	return NewTypeRef(t.InlinedName())
}

// UnmarshalJSON implements custom JSON deserialization logic.
func (t *TypeRef) UnmarshalJSON(b []byte) error {
	m := make(map[string]string)
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	if s, existed := m["$ref"]; existed && len(m) == 1 {
		*t = TypeRef(s)
		return nil
	}
	return fmt.Errorf(`Expect typeref to have only one "$ref" property saw %v`, m)
}

// MarshalJSON implements custom JSON serialization logic.
func (t *TypeRef) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"$ref": string(*t),
	})
}
