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

package build

import (
	"os"
	"path/filepath"
	"testing"

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
					"name": "myapp",
					"properties": {}
				}
			},
			"frontend": {
				"type": "Applications.Core/containers@2023-10-01-preview",
				"properties": {
					"name": "frontend",
					"properties": {
						"application": "[reference('app').id]",
						"container": {"image": "myregistry/frontend:latest"},
						"connections": {
							"backend": {"source": "[resourceId('Applications.Core/containers', 'backend')]"}
						},
						"codeReference": "src/frontend/index.ts#L1"
					}
				},
				"dependsOn": ["app"]
			},
			"backend": {
				"type": "Applications.Core/containers@2023-10-01-preview",
				"properties": {
					"name": "backend",
					"properties": {
						"application": "[reference('app').id]",
						"container": {"image": "myregistry/backend:latest"}
					}
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

	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0o644))
	require.NoError(t, os.WriteFile(bicepPath, []byte(bicepSource), 0o644))

	artifact, err := BuildStaticGraph(armPath, bicepPath)
	require.NoError(t, err)
	require.NotNil(t, artifact)

	assert.Equal(t, "1.0.0", artifact.Version)
	assert.Equal(t, "app.bicep", artifact.SourceFile)
	assert.Len(t, artifact.Application.Resources, 3)

	// Verify resources are sorted by ID.
	for i := 1; i < len(artifact.Application.Resources); i++ {
		assert.True(t,
			artifact.Application.Resources[i-1].ID < artifact.Application.Resources[i].ID,
			"resources should be sorted by ID")
	}

	// Every resource gets a diff hash.
	for _, r := range artifact.Application.Resources {
		assert.NotEmpty(t, r.DiffHash, "resource %s should have a diff hash", r.Name)
	}
}

func TestBuildStaticGraph_DependsOnEdgeExtraction(t *testing.T) {
	t.Parallel()

	armJSON := `{
		"resources": {
			"app": {
				"type": "Applications.Core/applications@2023-10-01-preview",
				"properties": {"name": "myapp", "properties": {}}
			},
			"frontend": {
				"type": "Applications.Core/containers@2023-10-01-preview",
				"properties": {
					"name": "frontend",
					"properties": {
						"connections": {
							"app": {"source": "[resourceId('Applications.Core/applications', 'myapp')]"}
						}
					}
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
}
`

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	bicepPath := filepath.Join(dir, "app.bicep")

	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0o644))
	require.NoError(t, os.WriteFile(bicepPath, []byte(bicepSource), 0o644))

	artifact, err := BuildStaticGraph(armPath, bicepPath)
	require.NoError(t, err)

	var frontend, app *Resource
	for _, r := range artifact.Application.Resources {
		switch r.Name {
		case "frontend":
			frontend = r
		case "myapp":
			app = r
		}
	}

	require.NotNil(t, frontend, "frontend resource should exist")
	require.NotNil(t, app, "app resource should exist")

	// Frontend should have at least one outbound connection (to the app).
	outbound := 0
	for _, c := range frontend.Connections {
		if c.Direction == DirectionOutbound {
			outbound++
		}
	}
	assert.GreaterOrEqual(t, outbound, 1, "frontend should have at least one outbound connection")

	// App should receive a reciprocal inbound edge.
	inbound := 0
	for _, c := range app.Connections {
		if c.Direction == DirectionInbound {
			inbound++
		}
	}
	assert.GreaterOrEqual(t, inbound, 1, "app should have at least one inbound connection")
}

func TestBuildStaticGraph_CodeReferencePassthrough(t *testing.T) {
	t.Parallel()

	armJSON := `{
		"resources": {
			"cache": {
				"type": "Applications.Datastores/redisCaches@2023-10-01-preview",
				"properties": {
					"name": "cache",
					"properties": {
						"codeReference": "src/cache/redis.ts#L10"
					}
				}
			}
		}
	}`

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	bicepPath := filepath.Join(dir, "app.bicep")

	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0o644))
	require.NoError(t, os.WriteFile(bicepPath, []byte(""), 0o644))

	artifact, err := BuildStaticGraph(armPath, bicepPath)
	require.NoError(t, err)
	require.Len(t, artifact.Application.Resources, 1)

	assert.Equal(t, "src/cache/redis.ts#L10", artifact.Application.Resources[0].CodeReference)
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

	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0o644))
	require.NoError(t, os.WriteFile(bicepPath, []byte(bicepSource), 0o644))

	artifact, err := BuildStaticGraph(armPath, bicepPath)
	require.NoError(t, err)

	for _, r := range artifact.Application.Resources {
		switch r.Name {
		case "myapp":
			assert.Equal(t, int32(2), r.AppDefinitionLine)
		case "frontend":
			assert.Equal(t, int32(6), r.AppDefinitionLine)
		}
	}
}

func TestBuildStaticGraph_MissingArmFile(t *testing.T) {
	t.Parallel()

	_, err := BuildStaticGraph("/nonexistent/path.json", "/nonexistent/app.bicep")
	assert.Error(t, err)
}

func TestBuildStaticGraph_InvalidArmJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	bicepPath := filepath.Join(dir, "app.bicep")

	require.NoError(t, os.WriteFile(armPath, []byte("{not-valid-json"), 0o644))
	require.NoError(t, os.WriteFile(bicepPath, []byte(""), 0o644))

	_, err := BuildStaticGraph(armPath, bicepPath)
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

func TestNormalizeSourceFilePath_RelativeToCwd(t *testing.T) {
	t.Parallel()

	// Absolute path inside the cwd → returns a slash-style relative path.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	got := normalizeSourceFilePath(filepath.Join(cwd, "sub", "app.bicep"))
	assert.Equal(t, "sub/app.bicep", got)
}

func TestNormalizeSourceFilePath_AbsoluteOutsideCwd_FallsBackToBase(t *testing.T) {
	t.Parallel()

	// Use an absolute path that is outside the cwd; expect just the basename.
	outside := filepath.Join(t.TempDir(), "other", "app.bicep")
	got := normalizeSourceFilePath(outside)
	assert.Equal(t, "app.bicep", got)
}

func TestResolveResourceIDExpression(t *testing.T) {
	t.Parallel()

	ids := map[string]string{}
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "bracketed",
			expr: "[resourceId('Applications.Core/containers', 'frontend')]",
			want: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend",
		},
		{
			name: "bare",
			expr: "resourceId('Applications.Core/containers', 'backend')",
			want: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/backend",
		},
		{
			name: "not-a-resource-id",
			expr: "reference('app').id",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, resolveResourceIDExpression(tc.expr, ids))
		})
	}
}
