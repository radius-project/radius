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
	"strings"
	"testing"

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/stretchr/testify/require"
)

func TestBuildModeledGraph_EmptyTemplate(t *testing.T) {
	t.Parallel()

	graph, err := BuildModeledGraph(map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, graph)
	require.NotNil(t, graph.Resources)
	require.Empty(t, graph.Resources)
}

func TestBuildModeledGraph_SkipsContainersAndRecipePacks(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{"type": "Applications.Core/applications", "name": "myapp"},
			map[string]any{"type": "Applications.Core/environments", "name": "myenv"},
			map[string]any{"type": "Radius.Core/recipePacks", "name": "mypack"},
			map[string]any{"type": "Applications.Core/containers", "name": "frontend",
				"properties": map[string]any{"image": "nginx"}},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	require.Len(t, graph.Resources, 1)
	require.Equal(t, "frontend", *graph.Resources[0].Name)
	require.Equal(t, "Applications.Core/containers", *graph.Resources[0].Type)
}

func TestBuildModeledGraph_BuildsResourceID(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type":       "Applications.Core/containers",
				"name":       "frontend",
				"properties": map[string]any{"image": "nginx"},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	require.Len(t, graph.Resources, 1)
	require.Equal(t,
		"/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend",
		*graph.Resources[0].ID,
	)
	require.Equal(t, "NotSpecified", *graph.Resources[0].ProvisioningState)
	require.True(t, strings.HasPrefix(*graph.Resources[0].DiffHash, "sha256:"))
}

func TestBuildModeledGraph_OutboundConnectionsResolved(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type": "Applications.Core/containers",
				"name": "frontend",
				"properties": map[string]any{
					"image": "nginx",
					"connections": map[string]any{
						"cache": map[string]any{
							"source": "[resourceId('Applications.Datastores/redisCaches', 'cache')]",
						},
					},
				},
			},
			map[string]any{
				"type":       "Applications.Datastores/redisCaches",
				"name":       "cache",
				"properties": map[string]any{},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	require.Len(t, graph.Resources, 2)

	frontend := findResource(t, graph, "frontend")
	require.Len(t, frontend.Connections, 1)
	require.Equal(t, corerpv20250801preview.DirectionOutbound, *frontend.Connections[0].Direction)
	require.Equal(t,
		"/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/redisCaches/cache",
		*frontend.Connections[0].ID,
	)
}

func TestBuildModeledGraph_InboundConnectionsAreReciprocal(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type": "Applications.Core/containers",
				"name": "frontend",
				"properties": map[string]any{
					"connections": map[string]any{
						"cache": map[string]any{
							"source": "[resourceId('Applications.Datastores/redisCaches', 'cache')]",
						},
					},
				},
			},
			map[string]any{
				"type":       "Applications.Datastores/redisCaches",
				"name":       "cache",
				"properties": map[string]any{},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	cache := findResource(t, graph, "cache")
	require.Len(t, cache.Connections, 1)
	require.Equal(t, corerpv20250801preview.DirectionInbound, *cache.Connections[0].Direction)
	require.Equal(t,
		"/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend",
		*cache.Connections[0].ID,
	)
}

func TestBuildModeledGraph_DropsUnresolvableConnections(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type": "Applications.Core/containers",
				"name": "frontend",
				"properties": map[string]any{
					"connections": map[string]any{
						"dyn": map[string]any{"source": "[parameters('something')]"},
					},
				},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	require.Empty(t, graph.Resources[0].Connections)
}

func TestBuildModeledGraph_DependsOnAffectsDiffHash(t *testing.T) {
	t.Parallel()

	withDep := map[string]any{
		"resources": []any{
			map[string]any{
				"type":       "Applications.Core/containers",
				"name":       "frontend",
				"properties": map[string]any{"image": "nginx"},
				"dependsOn":  []any{"[resourceId('Applications.Datastores/redisCaches', 'cache')]"},
			},
		},
	}
	withoutDep := map[string]any{
		"resources": []any{
			map[string]any{
				"type":       "Applications.Core/containers",
				"name":       "frontend",
				"properties": map[string]any{"image": "nginx"},
			},
		},
	}

	g1, err := BuildModeledGraph(withDep)
	require.NoError(t, err)
	g2, err := BuildModeledGraph(withoutDep)
	require.NoError(t, err)

	require.NotEqual(t, *g1.Resources[0].DiffHash, *g2.Resources[0].DiffHash)
}

func TestResolveResourceIDExpression(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"valid", "[resourceId('Applications.Core/containers', 'web')]",
			"/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/web"},
		{"empty", "", ""},
		{"non-resourceid", "[parameters('foo')]", ""},
		{"missing args", "[resourceId('only-one')]", ""},
	}
	for _, tc := range cases {
		got := resolveResourceIDExpression(tc.in)
		require.Equal(t, tc.want, got, tc.name)
	}
}

func findResource(t *testing.T, g *corerpv20250801preview.ApplicationGraphResponse, name string) *corerpv20250801preview.ApplicationGraphResource {
	t.Helper()
	for _, r := range g.Resources {
		if r != nil && r.Name != nil && *r.Name == name {
			return r
		}
	}
	t.Fatalf("resource %q not found", name)
	return nil
}

// TestBuildModeledGraph_PopulatesPropertiesAndDropsRuntimeKeys covers the
// baseline population contract: authored properties flow into the
// emitted graph node minus the runtime keys (provisioningState,
// connections, status) that the graph surfaces first-class. Matches
// the runtime graph's getResourceTypeSpecificProperties behavior so the
// two graphs compare cleanly.
func TestBuildModeledGraph_PopulatesPropertiesAndDropsRuntimeKeys(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type": "Radius.Data/postgreSqlDatabases",
				"name": "db",
				"properties": map[string]any{
					"database":          "app",
					"username":          "admin",
					"provisioningState": "Succeeded",
					"status":            map[string]any{"foo": "bar"},
					"connections":       map[string]any{},
				},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	require.Len(t, graph.Resources, 1)
	props := graph.Resources[0].Properties
	require.NotNil(t, props)
	require.Equal(t, "app", props["database"])
	require.Equal(t, "admin", props["username"])
	require.NotContains(t, props, "provisioningState")
	require.NotContains(t, props, "status")
	require.NotContains(t, props, "connections")
}

// TestBuildModeledGraph_NilPropertiesWhenAuthoredIsEmpty preserves the
// pre-population wire shape for resources that don't author anything:
// Properties stays nil rather than serializing as `{}`. Guards against
// noise in graph diffs when a resource genuinely has no inputs.
func TestBuildModeledGraph_NilPropertiesWhenAuthoredIsEmpty(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type":       "Applications.Core/containers",
				"name":       "empty",
				"properties": map[string]any{},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	require.Nil(t, graph.Resources[0].Properties)
}

// TestBuildModeledGraph_SecureStringDirectReference is the base case
// for rule A: a property value that is exactly `[parameters('name')]`
// where the referenced parameter is `secureString` gets nulled, while a
// sibling property that references a non-secure parameter is preserved.
func TestBuildModeledGraph_SecureStringDirectReference(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"parameters": map[string]any{
			"pw":     map[string]any{"type": "secureString"},
			"region": map[string]any{"type": "string"},
		},
		"resources": []any{
			map[string]any{
				"type": "Radius.Data/postgreSqlDatabases",
				"name": "db",
				"properties": map[string]any{
					"credentials": "[parameters('pw')]",
					"region":      "[parameters('region')]",
				},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	props := graph.Resources[0].Properties
	require.Nil(t, props["credentials"], "secureString-derived value must be nulled")
	require.Equal(t, "[parameters('region')]", props["region"])
}

// TestBuildModeledGraph_SecureStringNestedInsideExpression covers the
// coarse-match half of rule A: even when the secure parameter is
// embedded inside a larger expression (format/concat/etc), the whole
// resulting value is nulled. Trying to redact a substring inside the
// expression would produce an interpretable but broken value; nil is
// the safer signal.
func TestBuildModeledGraph_SecureStringNestedInsideExpression(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"parameters": map[string]any{
			"dbPassword": map[string]any{"type": "secureString"},
		},
		"resources": []any{
			map[string]any{
				"type": "Radius.Data/postgreSqlDatabases",
				"name": "db",
				"properties": map[string]any{
					"connectionURL": "[format('postgres://admin:{0}@host/db', parameters('dbPassword'))]",
				},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	require.Nil(t, graph.Resources[0].Properties["connectionURL"])
}

// TestBuildModeledGraph_SecureStringInNestedObjectAndArray verifies the
// walker descends into nested objects and array items when applying
// rule A. Common in real Bicep — credentials often nest inside
// `properties.auth.credentials.password` or similar.
func TestBuildModeledGraph_SecureStringInNestedObjectAndArray(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"parameters": map[string]any{
			"apiToken": map[string]any{"type": "secureString"},
		},
		"resources": []any{
			map[string]any{
				"type": "Custom.Provider/thing",
				"name": "x",
				"properties": map[string]any{
					"auth": map[string]any{
						"headers": []any{
							map[string]any{
								"name":  "Authorization",
								"value": "[format('Bearer {0}', parameters('apiToken'))]",
							},
						},
					},
				},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	auth := graph.Resources[0].Properties["auth"].(map[string]any)
	headers := auth["headers"].([]any)
	header0 := headers[0].(map[string]any)
	require.Equal(t, "Authorization", header0["name"])
	require.Nil(t, header0["value"], "secureString-derived value in array item must be nulled")
}

// TestBuildModeledGraph_SecureObjectReferenceAndFieldAccess covers the
// secureObject half of rule A: a `@secure() param blob object` compiles
// to `type: "secureObject"` in the ARM template. Whether the whole
// object is passed through (`[parameters('blob')]`) or a specific field
// is projected (`[parameters('blob').clientId]`), both must be nulled
// because the resulting value derives from a secret source. Mirrors the
// recipe driver's treatment of securestring / secureobject as
// equivalent for output sensitivity — see
// pkg/recipes/driver/bicep/bicep.go:isSecureARMOutputType.
func TestBuildModeledGraph_SecureObjectReferenceAndFieldAccess(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"parameters": map[string]any{
			"credentialsBlob": map[string]any{"type": "secureObject"},
		},
		"resources": []any{
			map[string]any{
				"type": "Custom.Provider/thing",
				"name": "x",
				"properties": map[string]any{
					"credentials":  "[parameters('credentialsBlob')]",
					"clientId":     "[parameters('credentialsBlob').clientId]",
					"clientSecret": "[parameters('credentialsBlob').clientSecret]",
					"tenantId":     "static-value",
				},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	props := graph.Resources[0].Properties
	require.Nil(t, props["credentials"], "whole secureObject reference must be nulled")
	require.Nil(t, props["clientId"], "field access on secureObject must be nulled")
	require.Nil(t, props["clientSecret"], "field access on secureObject must be nulled")
	require.Equal(t, "static-value", props["tenantId"], "non-secure values must pass through")
}

// TestBuildModeledGraph_NameBlocklistTopLevelAndNested covers rule B
// across every blocklisted key at both top level and one level deep.
// Case variants of the same name (Password / PASSWORD / password) all
// match — locking in the case-insensitivity contract.
func TestBuildModeledGraph_NameBlocklistTopLevelAndNested(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type": "Custom.Thing/instance",
				"name": "x",
				"properties": map[string]any{
					"password":         "hunter2",
					"Password":         "hunter2",
					"PASSWORD":         "hunter2",
					"connectionString": "server=x;pwd=y",
					"apiKey":           "k",
					"secret":           "s",
					"token":            "t",
					"privateKey":       "----BEGIN...",
					"sasToken":         "sv=2020",
					"database":         "app",
					"nested": map[string]any{
						"password": "still-secret",
						"public":   "ok",
					},
				},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	props := graph.Resources[0].Properties
	require.Nil(t, props["password"])
	require.Nil(t, props["Password"])
	require.Nil(t, props["PASSWORD"])
	require.Nil(t, props["connectionString"])
	require.Nil(t, props["apiKey"])
	require.Nil(t, props["secret"])
	require.Nil(t, props["token"])
	require.Nil(t, props["privateKey"])
	require.Nil(t, props["sasToken"])
	require.Equal(t, "app", props["database"])

	nested := props["nested"].(map[string]any)
	require.Nil(t, nested["password"], "blocklist must apply at every nesting depth")
	require.Equal(t, "ok", nested["public"])
}

// TestBuildModeledGraph_NameBlocklistIsExactMatch guards against
// over-eager matching. `passwordHash`, `apiKeyPolicy`, and
// `connectionStringTemplate` are legitimate non-sensitive keys in real
// Radius resource types and must not be redacted.
func TestBuildModeledGraph_NameBlocklistIsExactMatch(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type": "Custom.Thing/instance",
				"name": "x",
				"properties": map[string]any{
					"passwordHash":             "argon2-hash",
					"apiKeyPolicy":             "rotate",
					"connectionStringTemplate": "server={0}",
				},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	props := graph.Resources[0].Properties
	require.Equal(t, "argon2-hash", props["passwordHash"])
	require.Equal(t, "rotate", props["apiKeyPolicy"])
	require.Equal(t, "server={0}", props["connectionStringTemplate"])
}

// TestBuildModeledGraph_NameBlocklistNullsAnyValueType exercises the
// "regardless of value type" half of rule B. A blocklisted key holding
// an object or number gets nulled just as thoroughly as one holding a
// string, because the graph consumer must never see the concrete value.
func TestBuildModeledGraph_NameBlocklistNullsAnyValueType(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type": "Custom.Thing/instance",
				"name": "x",
				"properties": map[string]any{
					"secret": map[string]any{"raw": []byte("bin")},
					"token":  1234,
				},
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	props := graph.Resources[0].Properties
	require.Nil(t, props["secret"])
	require.Nil(t, props["token"])
}

// TestBuildModeledGraph_DiffHashIndependentOfRedaction pins the design
// note's claim that DiffHash is computed over authored properties
// pre-redaction: two graphs of the same authored app, one with a
// secure-param declaration and one without, must share the same
// DiffHash. Otherwise diff tooling would flap every time the
// sensitivity marker changed without a real content change.
func TestBuildModeledGraph_DiffHashIndependentOfRedaction(t *testing.T) {
	t.Parallel()

	baseResource := map[string]any{
		"type": "Radius.Data/postgreSqlDatabases",
		"name": "db",
		"properties": map[string]any{
			"database": "app",
			"password": "[parameters('pw')]",
		},
	}

	withSecure := map[string]any{
		"parameters": map[string]any{"pw": map[string]any{"type": "secureString"}},
		"resources":  []any{baseResource},
	}
	withoutSecure := map[string]any{
		"parameters": map[string]any{"pw": map[string]any{"type": "string"}},
		"resources":  []any{baseResource},
	}

	g1, err := BuildModeledGraph(withSecure)
	require.NoError(t, err)
	g2, err := BuildModeledGraph(withoutSecure)
	require.NoError(t, err)

	require.Equal(t, *g1.Resources[0].DiffHash, *g2.Resources[0].DiffHash,
		"DiffHash must be computed pre-redaction so schema-only changes do not flap the hash")

	// And confirm the visible Properties actually differ (rule B still
	// nulls `password` in both, but the secure-param one would null a
	// hypothetical differently-named property).
	require.Nil(t, g1.Resources[0].Properties["password"], "rule B nulls password by name")
	require.Nil(t, g2.Resources[0].Properties["password"], "rule B nulls password by name even without secure param")
}

// TestBuildModeledGraph_AuthoredMapNotMutated confirms the internal
// clone contract: the caller's input template survives untouched even
// after redaction runs on the emitted Properties bag. Enforced because
// ComputeDiffHash reads the same authored map and must not see redacted
// values.
func TestBuildModeledGraph_AuthoredMapNotMutated(t *testing.T) {
	t.Parallel()

	authored := map[string]any{
		"database": "app",
		"password": "hunter2",
		"nested":   map[string]any{"secret": "kept"},
	}
	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type":       "Custom.Thing/instance",
				"name":       "x",
				"properties": authored,
			},
		},
	}

	graph, err := BuildModeledGraph(template)
	require.NoError(t, err)
	// Graph copy is redacted...
	require.Nil(t, graph.Resources[0].Properties["password"])
	require.Nil(t, graph.Resources[0].Properties["nested"].(map[string]any)["secret"])
	// ...but the caller's original map is intact.
	require.Equal(t, "hunter2", authored["password"])
	require.Equal(t, "kept", authored["nested"].(map[string]any)["secret"])
}

// TestSensitiveParamNames covers the compiled-template scanner in
// isolation: only `secureString` and `secureObject` entries are surfaced
// (case-insensitively); malformed or missing parameter blocks return an
// empty set without error so downstream lookups (`_, ok := set[name]`)
// are safe.
func TestSensitiveParamNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		template map[string]any
		want     []string
	}{
		{
			name:     "no parameters block",
			template: map[string]any{},
			want:     nil,
		},
		{
			name: "mixed types",
			template: map[string]any{
				"parameters": map[string]any{
					"pw":     map[string]any{"type": "secureString"},
					"region": map[string]any{"type": "string"},
					"other":  map[string]any{"type": "int"},
					"caps":   map[string]any{"type": "SecureString"},
					"blob":   map[string]any{"type": "secureObject"},
					"blob2":  map[string]any{"type": "SECUREOBJECT"},
				},
			},
			want: []string{"blob", "blob2", "caps", "pw"},
		},
		{
			name: "malformed entries ignored",
			template: map[string]any{
				"parameters": map[string]any{
					"pw":  map[string]any{"type": "secureString"},
					"bad": "not-a-map",
				},
			},
			want: []string{"pw"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sensitiveParamNames(tc.template)
			gotKeys := make([]string, 0, len(got))
			for k := range got {
				gotKeys = append(gotKeys, k)
			}
			require.ElementsMatch(t, tc.want, gotKeys)
		})
	}
}

// TestContainsSecureParamReference exercises the substring matcher
// directly across the shapes real Bicep output can produce: direct
// references, format() wrappers, single vs double quotes, and
// non-expression strings that must never match.
func TestContainsSecureParamReference(t *testing.T) {
	t.Parallel()

	secureParams := map[string]struct{}{"pw": {}, "apiKey": {}}
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{"direct single quote", "[parameters('pw')]", true},
		{"direct double quote", `[parameters("pw")]`, true},
		{"nested format", "[format('user:{0}', parameters('pw'))]", true},
		{"other secure param", "[parameters('apiKey')]", true},
		{"non-secure param name", "[parameters('region')]", false},
		{"plain string literal", "hunter2", false},
		{"non-expression that happens to contain the substring",
			"parameters('pw')", false},
		{"empty", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, containsSecureParamReference(tc.s, secureParams))
		})
	}
}
