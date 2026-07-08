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
	// schema that declares which recipe outputs are secrets. Each sub-property of this block names
	// a secret key produced by the recipe (for example `connectionString`).
	SecretsBlockPropertyName = "secrets"

	// SecretReferencePropertyName is the name of the runtime, read-only reference property that
	// Radius populates on a resource instance. It points at the Radius.Security/secrets resource that
	// backs the resource's declared secrets so consumers can bind to it (for example via
	// `properties.secret.name`).
	SecretReferencePropertyName = "secret"
)

// GetSecretsBlock inspects a resource type OpenAPI schema and returns the names of the secret output
// properties declared under the top-level `secrets` object property. The second return value reports
// whether a `secrets` block is present at all. The returned keys are sorted for deterministic ordering.
//
// A resource type declares its recipe secret outputs like this:
//
//	properties:
//	  secrets:
//	    type: object
//	    readOnly: true
//	    properties:
//	      connectionString:
//	        type: string
//	        readOnly: true
//
// For the schema above GetSecretsBlock returns (["connectionString"], true).
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
	for key := range secretProps {
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
// The block must be an object marked readOnly, and every declared secret sub-property must be a readOnly
// string. It returns nil when no `secrets` block is declared.
func ValidateSecretsBlock(schema map[string]any) error {
	secretsSchema, ok := secretsBlockSchema(schema)
	if !ok {
		return nil
	}

	if t, ok := secretsSchema["type"].(string); ok && t != "object" {
		return fmt.Errorf("property '%s' must be an object", SecretsBlockPropertyName)
	}

	if readOnly, ok := secretsSchema["readOnly"].(bool); !ok || !readOnly {
		return fmt.Errorf("property '%s' must be marked readOnly", SecretsBlockPropertyName)
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
		if readOnly, ok := propSchema["readOnly"].(bool); !ok || !readOnly {
			return fmt.Errorf("secret '%s.%s' must be marked readOnly", SecretsBlockPropertyName, key)
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
