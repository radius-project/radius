// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"fmt"
	"testing"

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
		"ucp:/",
		"ucp:/planes",
		"ucp:/planes/radius/",
		"ucp:/planes/radius/local/resourceGroups//providers/Microsoft.CustomProviders/resourceProviders",
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
			id:       "ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers/test",
			expected: "ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers/test",
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
			id:       "ucp:/planes/radius/local/resourceGroups/r1/providers/Applications.Core/applications/cool-app",
			expected: "ucp:/planes/radius/local/resourceGroups/r1/providers/Applications.Core/applications/cool-app",
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
			id:       "ucp:/planes/radius/local/",
			expected: "ucp:/planes/radius/local",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
			},
			types:    []TypeSegment{},
			provider: "",
		},
		{
			id:       "ucp:/planes/radius/local/resourceGroups/r1",
			expected: "ucp:/planes/radius/local/resourceGroups/r1",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types:    []TypeSegment{},
			provider: "",
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

func Test_Append_Collection(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(TypeSegment{Name: "", Type: "test-resource"})
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource", appended.id)
}

func Test_Append_Collection_UCP(t *testing.T) {
	id, err := Parse("ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(TypeSegment{Name: "", Type: "test-resource"})
	require.Equal(t, "ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource", appended.id)
}

func Test_Append_NamedResource(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(TypeSegment{Name: "test-name", Type: "test-resource"})
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource/test-name", appended.id)
}

func Test_Append_NamedResource_UCP(t *testing.T) {
	id, err := Parse("ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(TypeSegment{Name: "test-name", Type: "test-resource"})
	require.Equal(t, "ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource/test-name", appended.id)
}

func Test_Truncate_Success(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.id)
}

func Test_Truncate_Success_UCP(t *testing.T) {
	id, err := Parse("ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.id)
}

func Test_Truncate_ReturnsSelfForTopLevelResource(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.id)
}

func Test_Truncate_ReturnsSelfForTopLevelResource_UCP(t *testing.T) {
	id, err := Parse("ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "ucp:/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.id)
}
