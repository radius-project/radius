// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/stretchr/testify/require"
)

func Test_ParseInvalidIDs(t *testing.T) {
	values := []string{
		"",
		"/subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/",
		"//subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/resourceProviders",
		"/subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/resourceProviders//",
		"/subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders//",
		"/subscriptions/{%s}/resourceGroups/{%s}/ddproviders/Microsoft.CustomProviders/resourceProviders",
		"/subscriptions/{%s}/resourceGroups//providers/Microsoft.CustomProviders/resourceProviders",
		"/subscriptions/{%s}/resourceGroups/providers/Microsoft.CustomProviders/resourceProviders",
		"/planes/radius",
		"/planes/radius/local/resourceGroups//providers/Microsoft.CustomProviders/resourceProviders",
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v), func(t *testing.T) {
			_, err := Parse(v)
			require.Errorf(t, err, "shouldn't have parsed %s", v)
		})
	}
}

func Test_ParseValidIDs(t *testing.T) {
	values := []struct {
		id       string
		expected string
		scopes   []ScopeSegment
		types    []TypeSegment
		provider string
	}{
		{
			id:       "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			scopes: []ScopeSegment{
				{Type: "subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/planes",
			expected: "/planes",
			scopes:   []ScopeSegment{},
			types:    []TypeSegment{},
			provider: "",
		},
		{
			id:       "/planes/",
			expected: "/planes",
			scopes:   []ScopeSegment{},
			types:    []TypeSegment{},
			provider: "",
		},
		{
			id:       "/",
			expected: "/",
			scopes:   []ScopeSegment{},
			types:    []TypeSegment{},
			provider: "",
		},
		{
			id:       "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			scopes: []ScopeSegment{
				{Type: "subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			scopes: []ScopeSegment{
				{Type: "subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			expected: "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			scopes: []ScopeSegment{
				{Type: "Subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/foo/bar",
			expected: "/Subscriptions/s1/resourceGroups/r1/providers/foo/bar",
			scopes: []ScopeSegment{
				{Type: "Subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "foo/bar"},
			},
			provider: "foo",
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius",
			expected: "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius",
			scopes: []ScopeSegment{
				{Type: "Subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/",
			expected: "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius",
			scopes: []ScopeSegment{
				{Type: "Subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/",
			expected: "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius",
			scopes: []ScopeSegment{
				{Type: "Subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications",
			expected: "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications",
			scopes: []ScopeSegment{
				{Type: "Subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
				{Type: "Applications"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app",
			expected: "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app",
			scopes: []ScopeSegment{
				{Type: "Subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
				{Type: "Applications", Name: "test-app"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers",
			expected: "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers",
			scopes: []ScopeSegment{
				{Type: "Subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
				{Type: "Applications", Name: "test-app"},
				{Type: "Containers"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers/test",
			expected: "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers/test",
			scopes: []ScopeSegment{
				{Type: "Subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
				{Type: "Applications", Name: "test-app"},
				{Type: "Containers", Name: "test"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers/test",
			expected: "/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers/test",
			scopes: []ScopeSegment{
				{Type: "azure", Name: "azurecloud"},
				{Type: "subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
				{Type: "Applications", Name: "test-app"},
				{Type: "Containers", Name: "test"},
			},
			provider: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			id:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/applications/cool-app",
			expected: "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/applications/cool-app",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "Applications.Core/applications", Name: "cool-app"},
			},
			provider: "Applications.Core",
		},
		{
			id:       "/planes/radius/local/",
			expected: "/planes/radius/local",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
			},
			types:    []TypeSegment{},
			provider: "",
		},
		{
			id:       "/planes/radius/local/resourceGroups/r1",
			expected: "/planes/radius/local/resourceGroups/r1",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types:    []TypeSegment{},
			provider: "",
		},
		{
			id:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			expected: "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{{
				Type: "Applications.Core/environments", Name: "env"},
			},
			provider: "Applications.Core",
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.id), func(t *testing.T) {
			id, err := Parse(v.id)
			require.NoError(t, err)

			require.Equalf(t, v.expected, id.id, "id comparison failed for %s", v.id)
			require.Equalf(t, v.scopes, id.scopeSegments, "scope comparison failed for %s", v.id)

			require.Lenf(t, id.typeSegments, len(v.types), "types comparison failed for %s", v.id)
			for i := range id.typeSegments {
				require.Equalf(t, v.types[i], id.typeSegments[i], "types comparison failed for %s", v.id)
			}
		})
	}
}

func Test_MakeRelativeID(t *testing.T) {
	values := []struct {
		expected string
		scopes   []ScopeSegment
		types    []TypeSegment
	}{
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar",
			scopes: []ScopeSegment{
				{Type: "subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "foo/bar"},
			},
		},
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar/radius",
			scopes: []ScopeSegment{
				{Type: "subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "foo/bar", Name: "radius"},
			},
		},
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar/radius/t1",
			scopes: []ScopeSegment{
				{Type: "subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "foo/bar", Name: "radius"},
				{Type: "t1"},
			},
		},
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar/radius/t1/n1/t2",
			scopes: []ScopeSegment{
				{Type: "subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "foo/bar", Name: "radius"},
				{Type: "t1", Name: "n1"},
				{Type: "t2"},
			},
		},
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar/radius/t1/n1/t2/n2",
			scopes: []ScopeSegment{
				{Type: "subscriptions", Name: "s1"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{
				{Type: "foo/bar", Name: "radius"},
				{Type: "t1", Name: "n1"},
				{Type: "t2", Name: "n2"},
			},
		},
	}

	for i, v := range values {
		scopes := []ScopeSegment{
			{Type: "subscriptions", Name: "s1"},
			{Type: "resourceGroups", Name: "r1"},
		}
		t.Run(fmt.Sprintf("%d: %v", i, v.expected), func(t *testing.T) {
			actual := MakeRelativeID(scopes, v.types...)
			require.Equal(t, v.expected, actual)
		})
	}
}

func Test_FindScope(t *testing.T) {
	type testcase struct {
		ID       string
		Segment  string
		Expected string
	}

	cases := []testcase{
		{
			ID:       "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app",
			Segment:  SubscriptionsSegment,
			Expected: "s1",
		},
		{
			ID:       "/subscriPtions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app",
			Segment:  SubscriptionsSegment,
			Expected: "s1",
		},
		{
			ID:       "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app",
			Segment:  ResourceGroupsSegment,
			Expected: "r1",
		},
		{
			ID:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			Segment:  "radius",
			Expected: "local",
		},
		{
			ID:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			Segment:  ResourceGroupsSegment,
			Expected: "r1",
		},
	}

	for _, tc := range cases {
		id, err := Parse(tc.ID)
		require.NoError(t, err)

		result := id.FindScope(tc.Segment)
		require.Equal(t, tc.Expected, result)
	}
}

func Test_Append_Collection(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(TypeSegment{Name: "", Type: "test-resource"})
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource", appended.id)
}

func Test_Append_Collection_UCP(t *testing.T) {
	id, err := Parse("/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(TypeSegment{Name: "", Type: "test-resource"})
	require.Equal(t, "/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource", appended.id)
}

func Test_Append_NamedResource(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(TypeSegment{Name: "test-name", Type: "test-resource"})
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource/test-name", appended.id)
}

func Test_Append_NamedResource_UCP(t *testing.T) {
	id, err := Parse("/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(TypeSegment{Name: "test-name", Type: "test-resource"})
	require.Equal(t, "/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource/test-name", appended.id)
}

func Test_Truncate_Success(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.id)
}

func Test_Truncate_Success_Scope(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/subscriptions/s1", truncated.id)
}

func Test_Truncate_Success_Scope_UCP(t *testing.T) {
	id, err := Parse("/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/planes/azure/azurecloud/subscriptions/s1", truncated.id)
}

func Test_Truncate_Success_UCP(t *testing.T) {
	id, err := Parse("/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.id)
}

func Test_Truncate_ReturnsSelfForTopLevelResource(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.id)
}

func Test_Truncate_ReturnsSelfForTopLevelResource_UCP(t *testing.T) {
	id, err := Parse("/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.id)
}

func Test_Truncate_ReturnsSelfForTopLevelScope_UCP(t *testing.T) {
	id, err := Parse("/planes/")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/planes", truncated.id)
}

func Test_Truncate_WithCustomAction(t *testing.T) {
	id, err := Parse("/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0/listSecrets")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0", truncated.id)
}

func Test_IdParsing_WithNoTypeSegments(t *testing.T) {
	idStr := "/planes/radius/local/"
	id, err := Parse(idStr)
	require.NoError(t, err, "URL parsing failed")

	provider := id.ProviderNamespace()
	require.Equal(t, "", provider)

	routingScope := id.RoutingScope()
	require.Equal(t, "", routingScope)
}

func TestPlaneScope(t *testing.T) {
	tests := []struct {
		desc       string
		id         string
		planeScope string
	}{
		{
			desc:       "Azure resource id",
			id:         "/subscriptions/s1/resourceGroups/r1/providers/Applications.Core/applications/cool-app",
			planeScope: "/subscriptions/s1",
		},
		{
			desc:       "UCP resource id",
			id:         "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/applications/cool-app",
			planeScope: "/planes/radius/local",
		},
		{
			desc:       "No subscription or plane level types",
			id:         "/resourceGroups/r1/providers/Applications.Core/applications/cool-app",
			planeScope: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			rID, err := Parse(tt.id)
			require.NoError(t, err)
			require.Equal(t, tt.planeScope, rID.PlaneScope())
		})
	}
}

func TestPlaneNamespace(t *testing.T) {
	tests := []struct {
		desc     string
		id       string
		parseErr bool
		plane    string
	}{
		{
			"empty-id",
			"",
			true,
			"",
		},
		{
			"arm-container-resource",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/test-container-0",
			false,
			"",
		},
		{
			"ucp-invalid-resource",
			"/planes/radius",
			true,
			"",
		},
		{
			"ucp-valid-resource",
			"/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/containers/test-container-0",
			false,
			"radius/local",
		},
		{
			"ucp-missing-plane-name",
			"/planes/radius/resourceGroups/radius-test-rg/providers/Applications.Core/containers/test-container-0",
			true,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			rID, err := Parse(tt.id)
			if tt.parseErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.plane, rID.PlaneNamespace())
		})
	}
}

func Test_ValidateResourceType_Valid(t *testing.T) {
	testID := ID{
		id: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-db",
		scopeSegments: []ScopeSegment{
			{Type: "subscriptions", Name: "s1"},
			{Type: "resourceGroups", Name: "r1"},
		},
		typeSegments: []TypeSegment{
			{Type: "Microsoft.DocumentDB/databaseAccounts", Name: "test-account"},
			{Type: "mongodbDatabases", Name: "test-db"},
		},
	}

	err := testID.ValidateResourceType(KnownType{Types: []TypeSegment{
		{
			Type: azresources.DocumentDBDatabaseAccounts,
			Name: "*",
		},
		{
			Type: azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
			Name: "*",
		},
	}})
	require.NoError(t, err)
}

func Test_ValidateResourceType_Invalid(t *testing.T) {
	values := []struct {
		testID        ID
		testKnownType KnownType
	}{
		{
			testID: ID{
				id: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
				scopeSegments: []ScopeSegment{
					{Type: "subscriptions", Name: "s1"},
					{Type: "resourceGroups", Name: "r1"},
				},
				typeSegments: []TypeSegment{
					{Type: "Microsoft.DocumentDB", Name: "test-account"},
				},
			},
			testKnownType: KnownType{Types: []TypeSegment{
				{
					Type: azresources.DocumentDBDatabaseAccounts,
					Name: "*",
				},
				{
					Type: azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
					Name: "*",
				},
			}},
		},
		{
			testID: ID{
				id: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.DocumentDB/mongodbDatabases/test-db",
				scopeSegments: []ScopeSegment{
					{Type: "subscriptions", Name: "s1"},
					{Type: "resourceGroups", Name: "r1"},
				},
				typeSegments: []TypeSegment{
					{Type: "Microsoft.DocumentDB", Name: ""},
					{Type: "mongodbDatabases", Name: "test-db"},
				},
			},
			testKnownType: KnownType{Types: []TypeSegment{
				{
					Type: azresources.DocumentDBDatabaseAccounts,
					Name: "*",
				},
				{
					Type: azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
					Name: "*",
				},
			}},
		},
		{
			testID: ID{
				id: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
				scopeSegments: []ScopeSegment{
					{Type: "subscriptions", Name: "s1"},
					{Type: "resourceGroups", Name: "r1"},
				},
				typeSegments: []TypeSegment{
					{Type: "Microsoft.DocumentDB/databaseAccounts", Name: "test-account"},
				},
			},
			testKnownType: KnownType{Types: []TypeSegment{
				{
					Type: azresources.DocumentDBDatabaseAccounts,
					Name: "",
				},
			}},
		},
		{
			testID: ID{
				id: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.DocumentDB/databaseAccounts/",
				scopeSegments: []ScopeSegment{
					{Type: "subscriptions", Name: "s1"},
					{Type: "resourceGroups", Name: "r1"},
				},
				typeSegments: []TypeSegment{
					{Type: "Microsoft.DocumentDB/databaseAccounts", Name: ""},
				},
			},
			testKnownType: KnownType{Types: []TypeSegment{
				{
					Type: azresources.DocumentDBDatabaseAccounts,
					Name: "test-account",
				},
			}},
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.testID.id), func(t *testing.T) {
			err := v.testID.ValidateResourceType(v.testKnownType)
			require.Errorf(t, err, "resource '%s' does not match the expected resource type %s", v.testID.id)
		})
	}
}

func Test_ParseByMethod(t *testing.T) {
	testCases := []struct {
		desc   string
		id     string
		method string
		err    bool
		eID    string
		eRType string
	}{
		{
			desc:   "ucp-post-with-custom-action",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0/listSecrets",
			method: http.MethodPost,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "ucp-get",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			method: http.MethodGet,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "ucp-list",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases",
			method: http.MethodGet,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "ucp-put",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			method: http.MethodPut,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "ucp-patch",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			method: http.MethodPatch,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "ucp-delete",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			method: http.MethodDelete,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		}, {
			desc:   "arm-post-with-custom-action",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0/listSecrets",
			method: http.MethodPost,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "arm-get",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			method: http.MethodGet,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "arm-list",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases",
			method: http.MethodGet,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "arm-put",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			method: http.MethodPut,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "arm-patch",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			method: http.MethodPatch,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		},
		{
			desc:   "arm-delete",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			method: http.MethodDelete,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Connector/mongoDatabases/mongo-database-0",
			eRType: "Applications.Connector/mongoDatabases",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			parsedID, err := ParseByMethod(tt.id, tt.method)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.eID, parsedID.String())
				require.Equal(t, tt.eRType, parsedID.Type())
			}
		})
	}
}

func Test_RadiusRPResource(t *testing.T) {
	values := []struct {
		testID   ID
		expected bool
	}{
		{
			testID: ID{
				id: "/subscriptions/s1/resourceGroups/r1/providers/Applications.Core/containers/test-container",
				scopeSegments: []ScopeSegment{
					{Type: "subscriptions", Name: "s1"},
					{Type: "resourceGroups", Name: "r1"},
				},
				typeSegments: []TypeSegment{
					{Type: "Applications.Core/containers", Name: "test-container"},
				},
			},
			expected: true,
		},
		{
			testID: ID{
				id: "/subscriptions/s1/resourceGroups/r1/providers/Applications.Connector/mongoDatabases/test-mongo",
				scopeSegments: []ScopeSegment{
					{Type: "subscriptions", Name: "s1"},
					{Type: "resourceGroups", Name: "r1"},
				},
				typeSegments: []TypeSegment{
					{Type: "Applications.Connector/mongoDatabases", Name: "test-mongo"},
				},
			},
			expected: true,
		},
		{
			testID: ID{
				id: "/subscriptions/s1/resourceGroups/r1/providers/Applications.foo/containers/test-container",
				scopeSegments: []ScopeSegment{
					{Type: "subscriptions", Name: "s1"},
					{Type: "resourceGroups", Name: "r1"},
				},
				typeSegments: []TypeSegment{
					{Type: "Applications.foo/containers", Name: "test-container"},
				},
			},
			expected: false,
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.testID.id), func(t *testing.T) {
			radiusResource := v.testID.IsRadiusRPResource()
			require.Equal(t, v.expected, radiusResource)
		})
	}
}
