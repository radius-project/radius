// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azresources

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
		"/subscriDtions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/resourceProviders/",
		"/subscriptions/{%s}/resourceGrdoups/{%s}/providers/Microsoft.CustomProviders/resourceProviders",
		"/subscriptions/{%s}/resourceGroups/{%s}/ddproviders/Microsoft.CustomProviders/resourceProviders",
		"/subscriptions/{%s}/resourceGroups//providers/Microsoft.CustomProviders/resourceProviders",
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
		types    []ResourceType
	}{
		{
			id:       "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders"},
			},
		},
		{
			id:       "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders"},
			},
		},
		{
			id:       "subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders"},
			},
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders"},
			},
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/foo/bar",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar",
			types: []ResourceType{
				{Type: "foo/bar"},
			},
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
			},
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
			},
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
			},
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
				{Type: "Applications"},
			},
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
				{Type: "Applications", Name: "test-app"},
			},
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
				{Type: "Applications", Name: "test-app"},
				{Type: "Containers"},
			},
		},
		{
			id:       "/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers/test",
			expected: "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/Containers/test",
			types: []ResourceType{
				{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radius"},
				{Type: "Applications", Name: "test-app"},
				{Type: "Containers", Name: "test"},
			},
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.id), func(t *testing.T) {
			id, err := Parse(v.id)
			require.NoError(t, err)

			require.Equalf(t, v.expected, id.ID, "id comparison failed for %s", v.id)
			require.Equalf(t, "s1", id.SubscriptionID, "subscription id comparison failed for %s", v.id)
			require.Equalf(t, "r1", id.ResourceGroup, "resource group comparison failed for %s", v.id)

			require.Lenf(t, id.Types, len(v.types), "types comparison failed for %s", v.id)
			for i := range id.Types {
				require.Equalf(t, v.types[i], id.Types[i], "types comparison failed for %s", v.id)
			}
		})
	}
}

func Test_MakeID(t *testing.T) {
	values := []struct {
		expected string
		types    []ResourceType
	}{
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar",
			types: []ResourceType{
				{Type: "foo/bar"},
			},
		},
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar/radius",
			types: []ResourceType{
				{Type: "foo/bar", Name: "radius"},
			},
		},
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar/radius/t1",
			types: []ResourceType{
				{Type: "foo/bar", Name: "radius"},
				{Type: "t1"},
			},
		},
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar/radius/t1/n1/t2",
			types: []ResourceType{
				{Type: "foo/bar", Name: "radius"},
				{Type: "t1", Name: "n1"},
				{Type: "t2"},
			},
		},
		{
			expected: "/subscriptions/s1/resourceGroups/r1/providers/foo/bar/radius/t1/n1/t2/n2",
			types: []ResourceType{
				{Type: "foo/bar", Name: "radius"},
				{Type: "t1", Name: "n1"},
				{Type: "t2", Name: "n2"},
			},
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.expected), func(t *testing.T) {
			actual := MakeID("s1", "r1", v.types[0], v.types[1:]...)
			require.Equal(t, v.expected, actual)
		})
	}
}

func Test_MakeCollectionURITemplate(t *testing.T) {
	values := []struct {
		types    KnownType
		expected string
	}{
		{
			types:    KnownType{Types: []ResourceType{{Name: "*", Type: baseResourceType}}},
			expected: "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroup}/providers/Microsoft.CustomProviders/resourceProviders",
		},
		{
			types:    KnownType{Types: []ResourceType{{Name: "*", Type: baseResourceType}, {Name: "*", Type: "foo"}}},
			expected: "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroup}/providers/Microsoft.CustomProviders/resourceProviders/{resourceName0}/foo",
		},
		{
			types:    KnownType{Types: []ResourceType{{Name: "*", Type: baseResourceType}, {Name: "*", Type: "foo"}, {Name: "*", Type: "bar"}}},
			expected: "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroup}/providers/Microsoft.CustomProviders/resourceProviders/{resourceName0}/foo/{resourceName1}/bar",
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.expected), func(t *testing.T) {
			actual := MakeCollectionURITemplate(v.types)
			require.Equal(t, v.expected, actual)
		})
	}
}

func Test_MakeResourceURITemplate(t *testing.T) {
	values := []struct {
		types    KnownType
		expected string
	}{
		{
			types:    KnownType{Types: []ResourceType{{Name: "*", Type: baseResourceType}}},
			expected: "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroup}/providers/Microsoft.CustomProviders/resourceProviders/{resourceName0}",
		},
		{
			types:    KnownType{Types: []ResourceType{{Name: "*", Type: baseResourceType}, {Name: "*", Type: "foo"}}},
			expected: "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroup}/providers/Microsoft.CustomProviders/resourceProviders/{resourceName0}/foo/{resourceName1}",
		},
		{
			types:    KnownType{Types: []ResourceType{{Name: "*", Type: baseResourceType}, {Name: "*", Type: "foo"}, {Name: "*", Type: "bar"}}},
			expected: "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroup}/providers/Microsoft.CustomProviders/resourceProviders/{resourceName0}/foo/{resourceName1}/bar/{resourceName2}",
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.expected), func(t *testing.T) {
			actual := MakeResourceURITemplate(v.types)
			require.Equal(t, v.expected, actual)
		})
	}
}

func Test_Append_Collection(t *testing.T) {
	id, err := Parse("/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(ResourceType{Name: "", Type: "test-resource"})
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource", appended.ID)
}

func Test_Append_NamedResource(t *testing.T) {
	id, err := Parse("/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	appended := id.Append(ResourceType{Name: "test-name", Type: "test-resource"})
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app/test-resource/test-name", appended.ID)
}

func Test_Truncate_Success(t *testing.T) {
	id, err := Parse("/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.ID)
}

func Test_Truncate_ReturnsSelfForTopLevelResource(t *testing.T) {
	id, err := Parse("/Subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius", truncated.ID)
}
