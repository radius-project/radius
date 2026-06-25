/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package baseresource declares the "Radius-aware" properties (application,
// environment, connections, codeReference) that every resource type schema
// inherits, and merges them into per-type schemas.
//
// The properties are declared once in base.yaml, embedded into the binary, and
// applied to each schema by Apply(). Inheritance is implicit: a resource type
// author writes only type-specific properties, and the common properties are
// merged in before validation and registration.
package baseresource

import (
	_ "embed"
	"fmt"
	"sort"

	yaml "github.com/goccy/go-yaml"
)

// baseYAML is the embedded canonical base resource manifest. It is the single
// source of truth for the common properties every resource type inherits.
//
//go:embed base.yaml
var baseYAML []byte

// BaseManifest is the in-memory, immutable representation of the embedded base
// resource manifest. Construct it once at service/command initialization with
// Load (or MustLoad) and reuse it for the lifetime of the process — its
// contents never change after construction.
type BaseManifest struct {
	// properties maps each common property name to its schema fragment.
	properties map[string]any
	// required lists the common properties that are mandatory on every type.
	required []string
}

// Load parses the embedded base.yaml into an immutable BaseManifest. The schema
// map it produces uses the same Go types (map[string]any, []any) that the
// manifest decoder produces for per-type schemas, so Apply is type-consistent.
//
// Callers should invoke Load once, at initialization, and hold the result.
func Load() (*BaseManifest, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(baseYAML, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse embedded base resource manifest: %w", err)
	}

	props, err := toStringMap(raw["properties"])
	if err != nil {
		return nil, fmt.Errorf("base resource manifest has invalid \"properties\": %w", err)
	}
	if len(props) == 0 {
		return nil, fmt.Errorf("base resource manifest declares no properties")
	}

	required, err := toStringSlice(raw["required"])
	if err != nil {
		return nil, fmt.Errorf("base resource manifest has invalid \"required\": %w", err)
	}

	return &BaseManifest{properties: props, required: required}, nil
}

// MustLoad is like Load but panics if the embedded base manifest cannot be
// parsed. The embedded manifest is a build-time constant validated by tests, so
// a failure indicates a programming error and should fail fast at startup. It
// is intended for package-level initialization (var base = MustLoad()).
func MustLoad() *BaseManifest {
	b, err := Load()
	if err != nil {
		panic(err)
	}
	return b
}

// RawManifest returns a copy of the embedded canonical base resource manifest
// YAML. It lets other packages — for example bicep-tools, which models schemas
// with its own types — decode the base property set from the same source of
// truth instead of duplicating the definitions.
func RawManifest() []byte {
	out := make([]byte, len(baseYAML))
	copy(out, baseYAML)
	return out
}

// Apply merges the common base properties into the given resource type schema.
//
// For every base property absent from the schema it copies the base definition
// in (per-type-wins precedence: an author's own declaration is never
// overwritten). It then unions the base "required" list into the schema's
// "required" list so mandatory common properties stay mandatory.
//
// Apply mutates schema in place. It is purely lexical — no network, file, or
// UCP access — and idempotent: applying it more than once yields the same
// result. A nil schema is a no-op.
func (b *BaseManifest) Apply(schema map[string]any) error {
	if schema == nil {
		return nil
	}

	// Merge absent base properties, copying so the schema owns its values.
	props, err := schemaProperties(schema)
	if err != nil {
		return err
	}
	for _, name := range sortedKeys(b.properties) {
		if _, exists := props[name]; exists {
			continue // per-type-wins: keep the author's declaration.
		}
		props[name] = deepCopy(b.properties[name])
	}
	schema["properties"] = props

	// Union the base required list into the schema's required list.
	required, err := toStringSlice(schema["required"])
	if err != nil {
		return fmt.Errorf("schema has invalid \"required\": %w", err)
	}
	present := make(map[string]bool, len(required))
	for _, name := range required {
		present[name] = true
	}
	for _, name := range b.required {
		if !present[name] {
			required = append(required, name)
			present[name] = true
		}
	}
	if len(required) > 0 {
		schema["required"] = toAnySlice(required)
	}

	return nil
}

// ConflictingProperties returns the names of base (common) properties that the
// given schema already declares under "properties", sorted. Because the base
// manifest owns these property names, a per-type schema must not redeclare
// them; callers use this to reject such schemas before merging.
//
// Listing a base property under "required" (without declaring it under
// "properties") is allowed and is not reported here. A nil schema, or one
// without a "properties" object, has no conflicts.
//
// ConflictingProperties must run on the author's raw schema before Apply: once
// Apply has injected the base properties they are indistinguishable from a
// redeclaration, so a post-merge check would report false positives.
func (b *BaseManifest) ConflictingProperties(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	props, err := schemaProperties(schema)
	if err != nil || len(props) == 0 {
		return nil
	}
	var conflicts []string
	for name := range b.properties {
		if _, exists := props[name]; exists {
			conflicts = append(conflicts, name)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

// PropertyNames returns the names of the common base properties, sorted.
func (b *BaseManifest) PropertyNames() []string {
	return sortedKeys(b.properties)
}

// RequiredNames returns the names of the base properties that are mandatory on
// every resource type, sorted.
func (b *BaseManifest) RequiredNames() []string {
	names := append([]string(nil), b.required...)
	sort.Strings(names)
	return names
}

// schemaProperties returns the schema's "properties" map, creating an empty one
// if absent. It errors if "properties" is present but not an object.
func schemaProperties(schema map[string]any) (map[string]any, error) {
	value, ok := schema["properties"]
	if !ok || value == nil {
		return map[string]any{}, nil
	}
	props, err := toStringMap(value)
	if err != nil {
		return nil, fmt.Errorf("schema has invalid \"properties\": %w", err)
	}
	return props, nil
}

// toStringMap coerces a decoded YAML/JSON value into a map[string]any.
func toStringMap(value any) (map[string]any, error) {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return typed, nil
	default:
		return nil, fmt.Errorf("expected an object, got %T", value)
	}
}

// toStringSlice coerces a decoded YAML/JSON list into a []string, accepting both
// []any (manifest decoder output) and []string (Go-constructed schemas).
func toStringSlice(value any) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected a list of strings, got element %T", item)
			}
			result = append(result, str)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected a list, got %T", value)
	}
}

// toAnySlice converts a []string back into the []any representation used by the
// decoded schema maps.
func toAnySlice(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

// sortedKeys returns the keys of m in sorted order for deterministic output.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// deepCopy returns a deep copy of a decoded YAML/JSON value so that merged base
// properties are owned by the target schema and never share mutable state.
func deepCopy(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, item := range typed {
			copied[key] = deepCopy(item)
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for i, item := range typed {
			copied[i] = deepCopy(item)
		}
		return copied
	default:
		return value
	}
}
