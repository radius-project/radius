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

package schema

import (
	"fmt"
	"sort"
)

const (
	// SecretsBlockPropertyName is the name of the top-level object property in a resource type
	// schema that declares a resource's recipe secrets. Its sub-properties are the reserved secret
	// reference (see SecretNameReferenceKey) plus one entry per secret key produced by the recipe
	// (for example `connectionString`).
	SecretsBlockPropertyName = "secrets"

	// SecretNameReferenceKey is the reserved sub-property of the `secrets` block that Radius populates
	// at runtime with the name of the managed Radius.Security/secrets resource backing the resource's
	// declared secrets. Consumers bind to it by name (for example via `properties.secrets.name`). It is
	// a reserved reference, not a materializable secret data key.
	SecretNameReferenceKey = "name"
)

// GetSecretsBlock inspects a resource type OpenAPI schema and returns the names of the recipe secret
// output keys declared under the top-level `secrets` object property. The second return value reports
// whether a `secrets` block is present at all. The returned keys are sorted for deterministic ordering.
//
// A resource type declares its recipe secret outputs like this:
//
//	properties:
//	  secrets:
//	    type: object
//	    properties:
//	      name:
//	        type: string
//	        readOnly: true
//	      connectionString:
//	        type: string
//	        readOnly: true
//
// For the schema above GetSecretsBlock returns (["connectionString"], true): the reserved `name`
// reference is excluded, and only readOnly sub-properties are returned (they are recipe secret
// outputs to materialize). Writable sub-properties are reserved for future secret inputs and are not
// returned here. The `secrets` block itself is intentionally not required to be readOnly.
func GetSecretsBlock(schema map[string]any) ([]string, bool) {
	secretsSchema, ok := secretsBlockSchema(schema)
	if !ok {
		return nil, false
	}

	secretProps, ok := secretsSchema["properties"].(map[string]any)
	if !ok {
		// A secrets block with no declared sub-properties still counts as present so callers can
		// distinguish "no secrets block" from "empty secrets block".
		return []string{}, true
	}

	keys := make([]string, 0, len(secretProps))
	for key, raw := range secretProps {
		// The reserved reference sub-property is not a materializable secret data key.
		if key == SecretNameReferenceKey {
			continue
		}
		// Only readOnly sub-properties are recipe secret outputs to materialize. Writable
		// sub-properties are reserved for future secret inputs and are skipped here.
		propSchema, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if readOnly, ok := propSchema["readOnly"].(bool); !ok || !readOnly {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, true
}

// HasSecretsBlock reports whether the given resource type schema declares a `secrets` block.
func HasSecretsBlock(schema map[string]any) bool {
	_, ok := secretsBlockSchema(schema)
	return ok
}

// ValidateSecretsBlock validates the shape of the `secrets` block in a resource type schema, if present.
// The block must be an object. Every declared sub-property must be a string. The reserved `name`
// reference sub-property, if present, must be readOnly. The block itself is intentionally not required
// to be readOnly, and data sub-properties are not required to be readOnly, so the block can hold both
// recipe secret outputs (readOnly) and, in future, writable secret inputs. It returns nil when no
// `secrets` block is declared.
func ValidateSecretsBlock(schema map[string]any) error {
	secretsSchema, ok := secretsBlockSchema(schema)
	if !ok {
		return nil
	}

	if t, ok := secretsSchema["type"].(string); ok && t != "object" {
		return fmt.Errorf("property '%s' must be an object", SecretsBlockPropertyName)
	}

	secretProps, ok := secretsSchema["properties"].(map[string]any)
	if !ok {
		return nil
	}

	for key, raw := range secretProps {
		propSchema, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("secret '%s.%s' must be a schema object", SecretsBlockPropertyName, key)
		}
		if t, ok := propSchema["type"].(string); ok && t != "string" {
			return fmt.Errorf("secret '%s.%s' must be a string", SecretsBlockPropertyName, key)
		}
		// The reserved reference sub-property is a server-populated output and must be readOnly.
		if key == SecretNameReferenceKey {
			if readOnly, ok := propSchema["readOnly"].(bool); !ok || !readOnly {
				return fmt.Errorf("secret '%s.%s' must be marked readOnly", SecretsBlockPropertyName, key)
			}
		}
	}

	return nil
}

// secretsBlockSchema returns the raw schema map for the top-level `secrets` object property, and whether
// it is present.
func secretsBlockSchema(schema map[string]any) (map[string]any, bool) {
	if schema == nil {
		return nil, false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, false
	}
	secretsSchema, ok := properties[SecretsBlockPropertyName].(map[string]any)
	if !ok {
		return nil, false
	}
	return secretsSchema, true
}
