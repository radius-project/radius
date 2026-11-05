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

	coredm "github.com/radius-project/radius/pkg/corerp/datamodel"
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
				"host":    "myhost.example.com",
				"port":    5432,
				"enabled": true,
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
			// An arbitrary fixture value, not derived from the hash rule. This
			// context is hand-built, so the resolver substitutes whatever is here.
			// The cases below reuse it only to stay self-consistent.
			ResourceNameHash: "b9f5a1471dadc792",
		},
		AWS: &recipecontext.ProviderAWS{
			Region:  "us-east-1",
			Account: "123456789",
			// An arbitrary fixture value, like the Azure hash above.
			ResourceNameHash: "4c1e0f2a9b7d3e56",
		},
	}
}

func Test_ResolveParameterExpressions(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]any
		ctx         *recipecontext.Context
		expected    map[string]any
		expectedErr string
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
			name: "derived Azure resource name hash resolves",
			params: map[string]any{
				"name": "redis-{{context.azure.resourceNameHash}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"name": "redis-b9f5a1471dadc792",
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
			name: "unknown context path fails",
			params: map[string]any{
				"value": "{{context.unknown.field}}",
			},
			ctx:         testContext(),
			expectedErr: `parameter "value": unresolved expression {{context.unknown.field}}: unknown context path "context.unknown.field"`,
		},
		{
			name: "non-context template expressions pass through unchanged",
			params: map[string]any{
				"helm":   "{{ .Values.name }}",
				"nested": "{{ .Values.context.name }}-{{context.resource.name}}",
				"plain":  "{{unknown}}",
				"open":   "prefix-{{ not closed",
			},
			ctx: testContext(),
			expected: map[string]any{
				"helm":   "{{ .Values.name }}",
				"nested": "{{ .Values.context.name }}-my-resource",
				"plain":  "{{unknown}}",
				"open":   "prefix-{{ not closed",
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
					"inner":  "{{context.resource.name}}",
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
			name: "nil context fails",
			params: map[string]any{
				"name": "{{context.resource.name}}",
			},
			ctx:         nil,
			expectedErr: `unknown context path "context.resource.name": recipe context is not available`,
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
			name: "single property expression preserves numeric type",
			params: map[string]any{
				"port": "{{context.resource.properties.port}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"port": 5432,
			},
		},
		{
			name: "single property expression preserves bool type",
			params: map[string]any{
				"enabled": "{{context.resource.properties.enabled}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"enabled": true,
			},
		},
		{
			name: "property expression interpolated into larger string is stringified",
			params: map[string]any{
				"label": "port-{{context.resource.properties.port}}",
			},
			ctx: testContext(),
			expected: map[string]any{
				"label": "port-5432",
			},
		},
		{
			name: "context.resource.properties missing property fails",
			params: map[string]any{
				"missing": "{{context.resource.properties.nonexistent}}",
			},
			ctx:         testContext(),
			expectedErr: `parameter "missing": unresolved expression {{context.resource.properties.nonexistent}}: expression evaluated to null`,
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
			t.Parallel()
			result, err := ResolveParameterExpressions(tt.params, tt.ctx)
			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func Test_ResolveParameterExpressions_GeneratedAzureResourceNameHash(t *testing.T) {
	ctx, err := recipecontext.New(
		&recipes.ResourceMetadata{
			ResourceID:    "/planes/radius/local/resourceGroups/testGroup/providers/applications.datastores/mongodatabases/mongo0",
			EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		},
		&recipes.Configuration{
			Providers: coredm.Providers{
				Azure: coredm.ProvidersAzure{
					Scope: "/subscriptions/testSub",
				},
			},
		},
	)
	require.NoError(t, err)

	result, err := ResolveParameterExpressions(
		map[string]any{"name": "redis-{{context.azure.resourceNameHash}}"},
		ctx,
	)
	require.NoError(t, err)

	// Assert against the context's own hash rather than a literal. What this test
	// owns is that the expression resolves to that field; whether the field itself
	// is correct is recipecontext's contract and is tested there.
	require.Equal(t, map[string]any{"name": "redis-" + ctx.Azure.ResourceNameHash}, result)
}

func Test_ResolveParameterExpressions_GeneratedAWSResourceNameHash(t *testing.T) {
	ctx, err := recipecontext.New(
		&recipes.ResourceMetadata{
			ResourceID:    "/planes/radius/local/resourceGroups/testGroup/providers/applications.datastores/mongodatabases/mongo0",
			EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		},
		&recipes.Configuration{
			Providers: coredm.Providers{
				AWS: coredm.ProvidersAWS{
					Scope: "/planes/aws/aws/accounts/123456789012/regions/us-west-2",
				},
			},
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, ctx.AWS.ResourceNameHash)

	result, err := ResolveParameterExpressions(
		map[string]any{"alias": "vpc-{{context.aws.resourceNameHash}}"},
		ctx,
	)
	require.NoError(t, err)

	require.Equal(t, map[string]any{"alias": "vpc-" + ctx.AWS.ResourceNameHash}, result)
}

func Test_TernaryExpressions(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]any
		ctx         *recipecontext.Context
		expected    map[string]any
		expectedErr string
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
			name: "ternary with unresolvable condition fails",
			params: map[string]any{
				"value": `{{context.unknown.path == "x" ? "yes" : "no"}}`,
			},
			ctx:         testContext(),
			expectedErr: `unknown context path "context.unknown.path"`,
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
			name: "chained ternary - first branch matches",
			params: map[string]any{
				"sku": `{{context.environment.name == "my-env" ? "Premium" : context.environment.name == "staging" ? "Standard" : "Basic"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"sku": "Premium",
			},
		},
		{
			name: "chained ternary - middle branch matches",
			params: map[string]any{
				"sku": `{{context.environment.name == "prod" ? "Premium" : context.environment.name == "my-env" ? "Standard" : "Basic"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"sku": "Standard",
			},
		},
		{
			name: "chained ternary - default branch matches",
			params: map[string]any{
				"sku": `{{context.environment.name == "prod" ? "Premium" : context.environment.name == "staging" ? "Standard" : "Basic"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"sku": "Basic",
			},
		},
		{
			name: "nested ternary in true arm",
			params: map[string]any{
				"value": `{{context.environment.name == "my-env" ? context.resource.name == "my-resource" ? "match" : "nomatch" : "other"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"value": "match",
			},
		},
		{
			name: "chained ternary with unresolvable condition on chosen branch fails",
			params: map[string]any{
				"value": `{{context.environment.name == "prod" ? "Premium" : context.unknown.path == "x" ? "Standard" : "Basic"}}`,
			},
			ctx:         testContext(),
			expectedErr: `unknown context path "context.unknown.path"`,
		},
		{
			name: "chained ternary with unresolvable condition on branch not taken resolves",
			params: map[string]any{
				"value": `{{context.environment.name == "my-env" ? "Premium" : context.unknown.path == "x" ? "Standard" : "Basic"}}`,
			},
			ctx: testContext(),
			expected: map[string]any{
				"value": "Premium",
			},
		},
		{
			name: "brace-wrapped nested ternary fails",
			params: map[string]any{
				// Nested ternaries must not be wrapped in their own {{...}}; the quotes no longer
				// balance, so the expression is malformed.
				"value": `{{context.environment.name == "a" ? "{{context.resource.name == "b" ? "c" : "d"}}" : "e"}}`,
			},
			ctx:         testContext(),
			expectedErr: `unsupported identifier "b"`,
		},
		{
			name: "three-way size mapping",
			params: map[string]any{
				"small":  `{{context.resource.properties.size == "S" ? "Standard_B1ms" : context.resource.properties.size == "M" ? "Standard_D2ds_v5" : "Standard_D4ds_v5"}}`,
				"medium": `{{context.resource.properties.tier == "S" ? "Standard_B1ms" : context.resource.properties.tier == "M" ? "Standard_D2ds_v5" : "Standard_D4ds_v5"}}`,
				"large":  `{{context.resource.properties.big == "S" ? "Standard_B1ms" : context.resource.properties.big == "M" ? "Standard_D2ds_v5" : "Standard_D4ds_v5"}}`,
			},
			ctx: func() *recipecontext.Context {
				ctx := testContext()
				ctx.Resource.Properties["size"] = "S"
				ctx.Resource.Properties["tier"] = "M"
				ctx.Resource.Properties["big"] = "L"
				return ctx
			}(),
			expected: map[string]any{
				"small":  "Standard_B1ms",
				"medium": "Standard_D2ds_v5",
				"large":  "Standard_D4ds_v5",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := ResolveParameterExpressions(tt.params, tt.ctx)
			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func Test_contextResolver(t *testing.T) {
	t.Parallel()

	t.Run("nil context rejects every path", func(t *testing.T) {
		t.Parallel()
		_, err := newContextResolver(nil).resolve("context.resource.properties.host")
		require.ErrorContains(t, err, "recipe context is not available")
	})

	t.Run("resolves all expected paths", func(t *testing.T) {
		t.Parallel()
		r := newContextResolver(testContext())
		expected := map[string]any{
			"context.resource.name":                                       "my-resource",
			"context.resource.type":                                       "Applications.Core/extenders",
			"context.azure.resourceNameHash":                              "b9f5a1471dadc792",
			"context.application.name":                                    "my-app",
			"context.environment.name":                                    "my-env",
			"context.runtime.kubernetes.namespace":                        "my-namespace",
			"context.runtime.kubernetes.environmentNamespace":             "my-env-namespace",
			"context.azure.resourceGroup.name":                            "my-rg",
			"context.azure.subscription.subscriptionId":                   "sub-id",
			"context.aws.region":                                          "us-east-1",
			"context.aws.account":                                         "123456789",
			"context.aws.resourceNameHash":                                "4c1e0f2a9b7d3e56",
			"context.resource.properties.host":                            "myhost.example.com",
			"context.resource.properties.port":                            5432,
			"context.resource.properties.enabled":                         true,
			"context.resource.connections.db.name":                        "my-db",
			"context.resource.connections.db.type":                        "Applications.Core/extenders",
			"context.resource.connections.db.properties.connectionString": "postgres://myhost:5432/mydb",
		}
		for path, want := range expected {
			got, err := r.resolve(path)
			require.NoError(t, err, path)
			assert.Equal(t, want, got, path)
		}
	})

	t.Run("absent properties and connections resolve to null", func(t *testing.T) {
		t.Parallel()
		r := newContextResolver(testContext())
		for _, path := range []string{
			"context.resource.properties.nonexistent",
			"context.resource.connections.db.properties.nonexistent",
			"context.resource.connections.missing.properties.host",
			"context.resource.connections.missing.name",
		} {
			got, err := r.resolve(path)
			require.NoError(t, err, path)
			assert.Nil(t, got, path)
		}
	})

	t.Run("unknown paths fail", func(t *testing.T) {
		t.Parallel()
		r := newContextResolver(testContext())
		for _, path := range []string{
			"context.unknown.field",
			"context.resource",
			"context.resource.connections.db",
			"context.resource.connections.db.unknown",
			"context.resource.connections.missing.unknown",
		} {
			_, err := r.resolve(path)
			require.ErrorContains(t, err, "unknown context path", path)
		}
	})

	t.Run("omits Azure resource name hash when not set", func(t *testing.T) {
		t.Parallel()
		ctx := testContext()
		ctx.Azure.ResourceNameHash = ""
		_, err := newContextResolver(ctx).resolve("context.azure.resourceNameHash")
		require.ErrorContains(t, err, "unknown context path")
	})

	t.Run("omits AWS resource name hash when not set", func(t *testing.T) {
		t.Parallel()
		ctx := testContext()
		ctx.AWS.ResourceNameHash = ""
		_, err := newContextResolver(ctx).resolve("context.aws.resourceNameHash")
		require.ErrorContains(t, err, "unknown context path")
	})

	t.Run("omits providers and runtime that are not configured", func(t *testing.T) {
		t.Parallel()
		ctx := testContext()
		ctx.Runtime.Kubernetes = nil
		ctx.Azure = nil
		ctx.AWS = nil
		r := newContextResolver(ctx)
		for _, path := range []string{
			"context.runtime.kubernetes.namespace",
			"context.azure.resourceGroup.name",
			"context.aws.region",
		} {
			_, err := r.resolve(path)
			require.ErrorContains(t, err, "unknown context path", path)
		}
	})
}
