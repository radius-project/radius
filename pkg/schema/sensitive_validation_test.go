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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeSensitiveEncryptedValues_StringPlaceholder(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"password": map[string]any{
				"type":                    "string",
				annotationRadiusSensitive: true,
			},
		},
	}

	properties := map[string]any{
		"password": map[string]any{
			"encrypted": "ciphertext",
			"nonce":     "nonce",
			"version":   1,
		},
	}

	sanitizeSensitiveEncryptedValues(properties, schema)

	value, ok := properties["password"]
	require.True(t, ok)
	require.Equal(t, "", value)
}

func TestSanitizeSensitiveEncryptedValues_ObjectPlaceholder(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"data": map[string]any{
				"type":                    "object",
				annotationRadiusSensitive: true,
				"properties": map[string]any{
					"value": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	properties := map[string]any{
		"data": map[string]any{
			"encrypted": "ciphertext",
			"nonce":     "nonce",
			"version":   1,
		},
	}

	sanitizeSensitiveEncryptedValues(properties, schema)

	value, ok := properties["data"]
	require.True(t, ok)
	require.Equal(t, map[string]any{}, value)
}

func TestSanitizeSensitiveEncryptedValues_MapValues(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"secretMap": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type":                    "string",
					annotationRadiusSensitive: true,
				},
			},
		},
	}

	properties := map[string]any{
		"secretMap": map[string]any{
			"first": map[string]any{
				"encrypted": "ciphertext",
				"nonce":     "nonce",
				"version":   1,
			},
			"second": "plain",
		},
	}

	sanitizeSensitiveEncryptedValues(properties, schema)

	secretMap, ok := properties["secretMap"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "", secretMap["first"])
	require.Equal(t, "plain", secretMap["second"])
}
