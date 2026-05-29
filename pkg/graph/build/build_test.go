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

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
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
						}
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

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0o644))

	resp, err := BuildStaticGraph(armPath)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Resources, 3)

	// Verify resources are sorted by ID.
	for i := 1; i < len(resp.Resources); i++ {
		assert.True(t,
			derefString(resp.Resources[i-1].ID) < derefString(resp.Resources[i].ID),
			"resources should be sorted by ID")
	}

	// Every resource has the expected basic fields populated.
	for _, r := range resp.Resources {
		assert.NotEmpty(t, derefString(r.ID))
		assert.NotEmpty(t, derefString(r.Type))
		assert.NotEmpty(t, derefString(r.Name))
		assert.Equal(t, provisioningStateSucceeded, derefString(r.ProvisioningState))
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

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(armPath, []byte(armJSON), 0o644))

	resp, err := BuildStaticGraph(armPath)
	require.NoError(t, err)

	var frontend, app *corerpv20250801preview.ApplicationGraphResource
	for _, r := range resp.Resources {
		switch derefString(r.Name) {
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
		if c.Direction != nil && *c.Direction == corerpv20250801preview.DirectionOutbound {
			outbound++
		}
	}
	assert.GreaterOrEqual(t, outbound, 1, "frontend should have at least one outbound connection")

	// App should receive a reciprocal inbound edge.
	inbound := 0
	for _, c := range app.Connections {
		if c.Direction != nil && *c.Direction == corerpv20250801preview.DirectionInbound {
			inbound++
		}
	}
	assert.GreaterOrEqual(t, inbound, 1, "app should have at least one inbound connection")
}

func TestBuildStaticGraph_MissingArmFile(t *testing.T) {
	t.Parallel()

	_, err := BuildStaticGraph("/nonexistent/path.json")
	assert.Error(t, err)
}

func TestBuildStaticGraph_InvalidArmJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	armPath := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(armPath, []byte("{not-valid-json"), 0o644))

	_, err := BuildStaticGraph(armPath)
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
