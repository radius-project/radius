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

package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildStaticGraph_ValidMultiResource(t *testing.T) {
	t.Parallel()

	armJSON := `{
		"$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
		"languageVersion": "1.9-experimental",
		"resources": {
			"app": {
				"type": "Applications.Core/applications@2023-10-01-preview",
				"properties": {
					"name": "myapp"
				}
			},
			"frontend": {
				"type": "Applications.Core/containers@2023-10-01-preview",
				"properties": {
					"name": "frontend",
					"application": "[reference('app').id]",
					"container": {"image": "myregistry/frontend:latest"},
					"connections": {
						"backend": {"source": "[resourceId('Applications.Core/containers', 'backend')]"}
					},
					"codeReference": "src/frontend/index.ts#L1"
				},
				"dependsOn": ["app"]
			},
			"backend": {
				"type": "Applications.Core/containers@2023-10-01-preview",
				"properties": {
					"name": "backend",
					"application": "[reference('app').id]",
					"container": {"image": "myregistry/backend:latest"}
				},
				"dependsOn": ["app"]
			}
		}
	}`

	bicepSource := `resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
}

resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'frontend'
  properties: {
    application: app.id
    container: { image: 'myregistry/frontend:latest' }
    codeReference: 'src/frontend/index.ts#L1'
  }
}

resource backend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'backend'
  properties: {
    application: app.id
    container: { image: 'myregistry/backend:latest' }
  }
}
`

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	bicepPath := filepath.Join(dir, "app.bicep")

	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0644))
	require.NoError(t, os.WriteFile(bicepPath, []byte(bicepSource), 0644))

	artifact, err := BuildStaticGraph(armPath, bicepPath)
	require.NoError(t, err)
	require.NotNil(t, artifact)

	assert.Equal(t, "1.0.0", artifact.Version)
	assert.Equal(t, "app.bicep", artifact.SourceFile)
	assert.Len(t, artifact.Application.Resources, 3)

	// Verify resources are sorted by ID.
	for i := 1; i < len(artifact.Application.Resources); i++ {
		assert.True(t,
			to.String(artifact.Application.Resources[i-1].ID) < to.String(artifact.Application.Resources[i].ID),
			"resources should be sorted by ID")
	}
}

func TestBuildStaticGraph_DependsOnEdgeExtraction(t *testing.T) {
	t.Parallel()

	armJSON := `{
		"resources": {
			"app": {
				"type": "Applications.Core/applications@2023-10-01-preview",
				"properties": {"name": "myapp"}
			},
			"frontend": {
				"type": "Applications.Core/containers@2023-10-01-preview",
				"properties": {"name": "frontend"},
				"dependsOn": ["app"]
			}
		}
	}`

	bicepSource := `resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
}
resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'frontend'
}
`

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	bicepPath := filepath.Join(dir, "app.bicep")

	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0644))
	require.NoError(t, os.WriteFile(bicepPath, []byte(bicepSource), 0644))

	artifact, err := BuildStaticGraph(armPath, bicepPath)
	require.NoError(t, err)

	// Find frontend resource.
	var frontend, app *json.RawMessage
	for _, r := range artifact.Application.Resources {
		if to.String(r.Name) == "frontend" {
			data, _ := json.Marshal(r)
			raw := json.RawMessage(data)
			frontend = &raw
		}
		if to.String(r.Name) == "myapp" {
			data, _ := json.Marshal(r)
			raw := json.RawMessage(data)
			app = &raw
		}
	}

	require.NotNil(t, frontend, "frontend resource should exist")
	require.NotNil(t, app, "app resource should exist")

	// Frontend should have an outbound connection to the app.
	var frontendRes map[string]interface{}
	require.NoError(t, json.Unmarshal(*frontend, &frontendRes))

	connections := frontendRes["connections"].([]interface{})
	assert.GreaterOrEqual(t, len(connections), 1, "frontend should have at least one connection")
}

func TestBuildStaticGraph_CodeReferencePassthrough(t *testing.T) {
	t.Parallel()

	armJSON := `{
		"resources": {
			"cache": {
				"type": "Applications.Datastores/redisCaches@2023-10-01-preview",
				"properties": {
					"name": "cache",
					"codeReference": "src/cache/redis.ts#L10"
				}
			}
		}
	}`

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	bicepPath := filepath.Join(dir, "app.bicep")

	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0644))
	require.NoError(t, os.WriteFile(bicepPath, []byte(""), 0644))

	artifact, err := BuildStaticGraph(armPath, bicepPath)
	require.NoError(t, err)
	require.Len(t, artifact.Application.Resources, 1)

	assert.Equal(t, "src/cache/redis.ts#L10", to.String(artifact.Application.Resources[0].CodeReference))
}

func TestBuildStaticGraph_SourceLineMapping(t *testing.T) {
	t.Parallel()

	armJSON := `{
		"resources": {
			"app": {
				"type": "Applications.Core/applications@2023-10-01-preview",
				"properties": {"name": "myapp"}
			},
			"frontend": {
				"type": "Applications.Core/containers@2023-10-01-preview",
				"properties": {"name": "frontend"}
			}
		}
	}`

	bicepSource := `// comment
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
}

resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'frontend'
}
`

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	bicepPath := filepath.Join(dir, "app.bicep")

	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0644))
	require.NoError(t, os.WriteFile(bicepPath, []byte(bicepSource), 0644))

	artifact, err := BuildStaticGraph(armPath, bicepPath)
	require.NoError(t, err)

	for _, r := range artifact.Application.Resources {
		switch to.String(r.Name) {
		case "myapp":
			require.NotNil(t, r.AppDefinitionLine)
			assert.Equal(t, int32(2), *r.AppDefinitionLine)
		case "frontend":
			require.NotNil(t, r.AppDefinitionLine)
			assert.Equal(t, int32(6), *r.AppDefinitionLine)
		}
	}
}

func TestBuildStaticGraph_MissingArmFile(t *testing.T) {
	t.Parallel()

	_, err := BuildStaticGraph("/nonexistent/path.json", "/nonexistent/app.bicep")
	assert.Error(t, err)
}

func TestExtractResourceType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"Applications.Core/containers@2023-10-01-preview", "Applications.Core/containers"},
		{"Applications.Core/applications", "Applications.Core/applications"},
		{"Applications.Datastores/redisCaches@2023-10-01-preview", "Applications.Datastores/redisCaches"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, extractResourceType(tc.input))
		})
	}
}
