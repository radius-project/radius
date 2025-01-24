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

package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadFileYAML(t *testing.T) {
	expected := &ResourceProvider{
		Name: "MyCompany.Resources",
		Location: map[string]string{
			"global": "http://localhost:8080",
		},
		Types: map[string]*ResourceType{
			"testResources": {
				APIVersions: map[string]*ResourceTypeAPIVersion{
					"2025-01-01-preview": {
						Schema: map[string]any{},
					},
				},
				Capabilities: []string{"SupportsRecipes"},
			},
		},
	}

	result, err := ReadFile("testdata/valid.yaml")
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestReadFile_InvalidYAML(t *testing.T) {
	// Errors in the yaml library are non-exported, so it's hard to test the exact error.
	result, err := ReadFile("testdata/invalid-yaml.yaml")
	require.Error(t, err)
	require.Nil(t, result)
}

func TestReadFile_DuplicateKeyYAML(t *testing.T) {
	// Errors in the yaml library are non-exported, so it's hard to test the exact error.
	result, err := ReadFile("testdata/duplicate-key.yaml")
	require.Error(t, err)
	require.Nil(t, result)
}

func TestReadFile_MissingRequiredFieldYAML(t *testing.T) {
	// Errors in the yaml library are non-exported, so it's hard to test the exact error.
	result, err := ReadFile("testdata/missing-required-field.yaml")
	require.Error(t, err)
	require.Nil(t, result)
}

func TestReadFileJSON(t *testing.T) {
	expected := &ResourceProvider{
		Name: "MyCompany.Resources",
		Types: map[string]*ResourceType{
			"testResources": {
				APIVersions: map[string]*ResourceTypeAPIVersion{
					"2025-01-01-preview": {
						Schema: map[string]any{},
					},
				},
				Capabilities: []string{"SupportsRecipes"},
			},
		},
	}

	result, err := ReadFile("testdata/valid.json")
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestReadFile_MissingRequiredFieldJSON(t *testing.T) {
	// Errors in the yaml library are non-exported, so it's hard to test the exact error.
	result, err := ReadFile("testdata/missing-required-field.json")
	require.Error(t, err)
	require.Nil(t, result)
}
