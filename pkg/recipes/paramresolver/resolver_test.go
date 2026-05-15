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

package paramresolver

import (
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testContext() *recipecontext.Context {
	return &recipecontext.Context{
		Resource: recipecontext.Resource{
			ResourceInfo: recipecontext.ResourceInfo{
				Name: "my-resource",
				ID:   "/planes/radius/local/resourceGroups/test/providers/Applications.Core/extenders/my-resource",
			},
			Type: "Applications.Core/extenders",
			Properties: map[string]any{
				"host": "myhost.example.com",
				"port": 5432,
			},
			Connections: map[string]recipes.ConnectedResource{
				"db": {
					ID:   "/planes/radius/local/resourceGroups/test/providers/Applications.Core/extenders/my-db",
					Name: "my-db",
					Type: "Applications.Core/extenders",
					Properties: map[string]any{
						"connectionString": "postgres://myhost:5432/mydb",
					},
				},
			},
		},
		Application: recipecontext.ResourceInfo{
			Name: "my-app",
			ID:   "/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/my-app",
		},
		Environment: recipecontext.ResourceInfo{
			Name: "my-env",
			ID:   "/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/my-env",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            "my-namespace",
				EnvironmentNamespace: "my-env-namespace",
			},
		},
		Azure: &recipecontext.ProviderAzure{
			ResourceGroup: recipecontext.AzureResourceGroup{
				Name: "my-rg",
				ID:   "/subscriptions/sub-id/resourceGroups/my-rg",
			},
			Subscription: recipecontext.AzureSubscription{
				SubscriptionID: "sub-id",
				ID:             "/subscriptions/sub-id",
			},
		},
		AWS: &recipecontext.ProviderAWS{
			Region:  "us-east-1",
			Account: "123456789",
		},
	}
}

func Test_ResolveParameterExpressions(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		ctx      *recipecontext.Context
		expected map[string]any
	}{
		{
			name:     "nil params returns nil",
			params:   nil,
			ctx:      testContext(),
			expected: nil,
		},
		{
			name:     "empty map returns empty map",
			params:   map[string]any{},
			ctx:      testContext(),
			expected: map[string]any{},
		},
		{
			name: "single expression resolves",
			params: map[string]any{
				"name": "{{context.resource.name}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"name": "my-resource",
			},
		},
		{
			name: "multiple expressions in one value",
			params: map[string]any{
				"tag": "{{context.application.name}}-{{context.environment.name}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"tag": "my-app-my-env",
			},
		},
		{
			name: "mixed literal and expression",
			params: map[string]any{
				"name": "prefix-{{context.resource.name}}-suffix",
			},
			ctx: testContext(),
			expected: map[string]any{
				"name": "prefix-my-resource-suffix",
			},
		},
		{
			name: "unrecognized expression left as-is",
			params: map[string]any{
				"value": "{{context.unknown.field}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"value": "{{context.unknown.field}}",
			},
		},
		{
			name: "non-string values pass through",
			params: map[string]any{
				"count":   42,
				"enabled": true,
				"ratio":   3.14,
			},
			ctx: testContext(),
			expected: map[string]any{
				"count":   42,
				"enabled": true,
				"ratio":   3.14,
			},
		},
		{
			name: "nested map traversal",
			params: map[string]any{
				"outer": map[string]any{
					"inner": "{{context.resource.name}}",
					"static": "no-change",
				},
			},
			ctx: testContext(),
			expected: map[string]any{
				"outer": map[string]any{
					"inner":  "my-resource",
					"static": "no-change",
				},
			},
		},
		{
			name: "slice values resolved",
			params: map[string]any{
				"tags": []any{"{{context.resource.name}}", "static-tag"},
			},
			ctx: testContext(),
			expected: map[string]any{
				"tags": []any{"my-resource", "static-tag"},
			},
		},
		{
			name: "nil context returns expressions as-is",
			params: map[string]any{
				"name": "{{context.resource.name}}",
			},
			ctx: nil,
			expected: map[string]any{
				"name": "{{context.resource.name}}",
			},
		},
		{
			name: "kubernetes runtime fields resolve",
			params: map[string]any{
				"namespace": "{{context.runtime.kubernetes.namespace}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"namespace": "my-namespace",
			},
		},
		{
			name: "azure provider fields resolve",
			params: map[string]any{
				"rg": "{{context.azure.resourceGroup.name}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"rg": "my-rg",
			},
		},
		{
			name: "aws provider fields resolve",
			params: map[string]any{
				"region": "{{context.aws.region}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"region": "us-east-1",
			},
		},
		{
			name: "context.resource.properties resolves existing property",
			params: map[string]any{
				"host": "{{context.resource.properties.host}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"host": "myhost.example.com",
			},
		},
		{
			name: "context.resource.properties resolves numeric property as string",
			params: map[string]any{
				"port": "{{context.resource.properties.port}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"port": "5432",
			},
		},
		{
			name: "context.resource.properties missing property left as-is",
			params: map[string]any{
				"missing": "{{context.resource.properties.nonexistent}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"missing": "{{context.resource.properties.nonexistent}}",
			},
		},
		{
			name: "multiple property expressions in one string",
			params: map[string]any{
				"url": "{{context.resource.properties.host}}:{{context.resource.properties.port}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"url": "myhost.example.com:5432",
			},
		},
		{
			name: "connection property resolves",
			params: map[string]any{
				"connStr": "{{context.resource.connections.db.properties.connectionString}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"connStr": "postgres://myhost:5432/mydb",
			},
		},
		{
			name: "connection metadata resolves",
			params: map[string]any{
				"dbName": "{{context.resource.connections.db.name}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"dbName": "my-db",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveParameterExpressions(tt.params, tt.ctx)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func Test_TernaryExpressions(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		ctx      *recipecontext.Context
		expected map[string]any
	}{
		{
			name: "ternary true branch",
			params: map[string]any{
				"sku": `{{context.environment.name == "my-env" ? "Standard" : "Basic"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"sku": "Standard",
			},
		},
		{
			name: "ternary false branch",
			params: map[string]any{
				"sku": `{{context.environment.name == "production" ? "Premium" : "Basic"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"sku": "Basic",
			},
		},
		{
			name: "ternary with context property in condition",
			params: map[string]any{
				"tier": `{{context.resource.properties.host == "myhost.example.com" ? "dedicated" : "shared"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"tier": "dedicated",
			},
		},
		{
			name: "ternary with unresolvable condition left as-is",
			params: map[string]any{
				"value": `{{context.unknown.path == "x" ? "yes" : "no"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"value": `{{context.unknown.path == "x" ? "yes" : "no"}}`,
			},
		},
		{
			name: "mixed ternary and literal text",
			params: map[string]any{
				"label": `env-{{context.environment.name == "my-env" ? "dev" : "prod"}}-ready`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"label": "env-dev-ready",
			},
		},
		{
			name: "nested/chained ternary left as-is (out of scope)",
			params: map[string]any{
				// This doesn't match ternary pattern because quotes contain nested ternary
				"value": `{{context.environment.name == "a" ? "{{context.resource.name == "b" ? "c" : "d"}}" : "e"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				// Inner braces break the outer regex match — left as-is
				"value": `{{context.environment.name == "a" ? "{{context.resource.name == "b" ? "c" : "d"}}" : "e"}}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveParameterExpressions(tt.params, tt.ctx)
			require.Equal(t, tt.expected, result)
		})
	}
}

func Test_buildContextLookup(t *testing.T) {
	t.Run("nil context returns empty map", func(t *testing.T) {
		lookup := buildContextLookup(nil)
		assert.Empty(t, lookup)
	})

	t.Run("populates all expected keys", func(t *testing.T) {
		ctx := testContext()
		lookup := buildContextLookup(ctx)

		assert.Equal(t, "my-resource", lookup["context.resource.name"])
		assert.Equal(t, "my-app", lookup["context.application.name"])
		assert.Equal(t, "my-env", lookup["context.environment.name"])
		assert.Equal(t, "my-namespace", lookup["context.runtime.kubernetes.namespace"])
		assert.Equal(t, "my-rg", lookup["context.azure.resourceGroup.name"])
		assert.Equal(t, "us-east-1", lookup["context.aws.region"])
		assert.Equal(t, "myhost.example.com", lookup["context.resource.properties.host"])
		assert.Equal(t, "5432", lookup["context.resource.properties.port"])
		assert.Equal(t, "my-db", lookup["context.resource.connections.db.name"])
		assert.Equal(t, "postgres://myhost:5432/mydb", lookup["context.resource.connections.db.properties.connectionString"])
	})

	t.Run("handles nil kubernetes runtime", func(t *testing.T) {
		ctx := testContext()
		ctx.Runtime.Kubernetes = nil
		lookup := buildContextLookup(ctx)
		_, ok := lookup["context.runtime.kubernetes.namespace"]
		assert.False(t, ok)
	})

	t.Run("handles nil azure provider", func(t *testing.T) {
		ctx := testContext()
		ctx.Azure = nil
		lookup := buildContextLookup(ctx)
		_, ok := lookup["context.azure.resourceGroup.name"]
		assert.False(t, ok)
	})

	t.Run("handles nil aws provider", func(t *testing.T) {
		ctx := testContext()
		ctx.AWS = nil
		lookup := buildContextLookup(ctx)
		_, ok := lookup["context.aws.region"]
		assert.False(t, ok)
	})
}
