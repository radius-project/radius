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

	yaml "github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/to"
)

func TestReadFileYAML(t *testing.T) {
	expected := &ResourceProvider{
		Namespace: "MyCompany.Resources",
		Location: map[string]string{
			"global": "http://localhost:8080",
		},
		Types: map[string]*ResourceType{
			"testResources": {
				Description: new("This is a test resource type."),
				APIVersions: map[string]*ResourceTypeAPIVersion{
					"2025-01-01-preview": {
						Schema: map[string]any{},
					},
				},
				Capabilities: []string{"ManualResourceProvisioning"},
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
		Namespace: "MyCompany.Resources",
		Types: map[string]*ResourceType{
			"testResources": {
				APIVersions: map[string]*ResourceTypeAPIVersion{
					"2025-01-01-preview": {
						Schema: map[string]any{},
					},
				},
				Capabilities: []string{"ManualResourceProvisioning"},
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

func TestResourceType_IconOmittedFromYAML(t *testing.T) {
	svg := "<svg xmlns=\"http://www.w3.org/2000/svg\"><rect/></svg>"
	rt := ResourceType{
		Description: to.Ptr("desc"),
		Icon:        to.Ptr(svg),
	}

	out, err := yaml.Marshal(rt)
	require.NoError(t, err)
	require.NotContains(t, string(out), "icon:", "Icon must not be serialized to YAML")
	require.NotContains(t, string(out), "<svg", "SVG bytes must not leak into YAML output")
}

func TestReadFile_IconKeyInYAMLRejected(t *testing.T) {
	data := []byte(`namespace: MyCompany.Resources
types:
  testResources:
    icon: "<svg/>"
    apiVersions:
      '2025-01-01-preview':
        schema: {}
    capabilities: ["ManualResourceProvisioning"]
`)

	result, err := ReadBytes(data)
	require.Error(t, err)
	require.Nil(t, result)
}
