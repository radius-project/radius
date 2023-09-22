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

package resources

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/radius-project/radius/pkg/azure/azresources"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	"github.com/stretchr/testify/require"
)

func Test_ParseInvalidIDs(t *testing.T) {
	values := []string{
		"",
		"invalid",
		"//",
		"//example.com",
		"subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/",
		"/subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/",
		"//subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/resourceProviders",
		"/subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/resourceProviders//",
		"/subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders//",
		"/subscriptions/{%s}/resourceGroups//providers/Microsoft.CustomProviders/resourceProviders",
		"/subscriptions/{%s}/resourceGroups/providers/Microsoft.CustomProviders/resourceProviders",
		"/subscriptions/{%s}/resourceGroups/providers",
		"planes/radius/local/resourceGroups/test-rg/providers/Applications.Test/testType",

		// Missing extension type
		"/planes/radius/local/resourceGroups/test-rg/providers/Applications.Test/testType/testResource/providers",
		"/planes/radius/local/resourceGroups/test-rg/providers/Applications.Test/testType/testResource/providers/Some.Extension",
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v), func(t *testing.T) {
			_, err := Parse(v)
			require.Errorf(t, err, "shouldn't have parsed %s", v)
		})
	}
}

type idkind string

const (
	kindnone                idkind = "none"
	kindscope               idkind = "scope"
	kindscopecollection     idkind = "scopecollection"
	kindresource            idkind = "resource"
	kindresourcecollection  idkind = "resourcecollection"
	kindextension           idkind = "extension"
	kindextensioncollection idkind = "extensioncollection"
)

func Test_ParseValidIDs(t *testing.T) {
	values := []struct {
		id             string
		expected       string
		rootScope      string
		routingScope   string
		parentResource string
		scopes         []ScopeSegment
		types          []TypeSegment
		extensions     []TypeSegment
		kind           idkind
		provider       string
		typeName       string
		name           string
		qualifiedName  string
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresourcecollection,
		},
		{
			id:       "/planes",
			expected: "/planes",
			provider: "",
			kind:     kindscope,
		},
		{
			id:       "/planes/",
			expected: "/planes",
			provider: "",
			kind:     kindscope,
		},
		{
			id:       "/",
			expected: "/",
			provider: "",
			kind:     kindscope,
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresourcecollection,
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresourcecollection,
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
			kind:     kindresourcecollection,
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresource,
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresource,
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresource,
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresourcecollection,
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
			provider:      "Microsoft.CustomProviders",
			kind:          kindresource,
			typeName:      "Microsoft.CustomProviders/resourceProviders/Applications",
			name:          "test-app",
			qualifiedName: "radius/test-app",
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresourcecollection,
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresource,
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
			provider: "Microsoft.CustomProviders",
			kind:     kindresource,
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
			kind:     kindresource,
		},
		{
			id:       "/planes",
			expected: "/planes",
			provider: "",
			kind:     kindscope,
		},
		{
			id:       "/planes/test",
			expected: "/planes/test",
			scopes: []ScopeSegment{
				{
					Type: "test",
				},
			},
			provider: "",
			kind:     kindscopecollection,
		},
		{
			id:       "/planes/radius/local/",
			expected: "/planes/radius/local",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
			},
			provider: "",
			kind:     kindscope,
		},
		{
			id:       "/planes/radius/local/resourceGroups/r1",
			expected: "/planes/radius/local/resourceGroups/r1",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
			},
			provider:      "",
			kind:          kindscope,
			typeName:      "System.Resources/resourceGroups",
			name:          "r1",
			qualifiedName: "local/r1",
		},
		{
			id:       "/planes/radius/local/resourceGroups/r1/resources",
			expected: "/planes/radius/local/resourceGroups/r1/resources",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
				{Type: "resources", Name: ""},
			},
			provider: "",
			kind:     kindscopecollection,
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
			kind:     kindresource,
		},
		{
			id:             "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env/providers/Some.Extension/extensionType",
			expected:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env/providers/Some.Extension/extensionType",
			rootScope:      "/planes/radius/local/resourceGroups/r1",
			routingScope:   "Some.Extension/extensionType",
			parentResource: "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{{
				Type: "Applications.Core/environments", Name: "env"},
			},
			extensions: []TypeSegment{{
				Type: "Some.Extension/extensionType", Name: "",
			}},
			provider:      "Some.Extension",
			kind:          kindextensioncollection,
			typeName:      "Some.Extension/extensionType",
			name:          "",
			qualifiedName: "",
		},
		{
			id:             "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env/providers/Some.Extension/extensionType/extensionResource",
			expected:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env/providers/Some.Extension/extensionType/extensionResource",
			rootScope:      "/planes/radius/local/resourceGroups/r1",
			routingScope:   "Some.Extension/extensionType/extensionResource",
			parentResource: "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{{
				Type: "Applications.Core/environments", Name: "env"},
			},
			extensions: []TypeSegment{{
				Type: "Some.Extension/extensionType", Name: "extensionResource",
			}},
			provider:      "Some.Extension",
			kind:          kindextension,
			typeName:      "Some.Extension/extensionType",
			name:          "extensionResource",
			qualifiedName: "extensionResource",
		},
		{
			id:             "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env/providers/Some.Extension/extensionType/extensionResource/anotherType",
			expected:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env/providers/Some.Extension/extensionType/extensionResource/anotherType",
			rootScope:      "/planes/radius/local/resourceGroups/r1",
			routingScope:   "Some.Extension/extensionType/extensionResource/anotherType",
			parentResource: "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{{
				Type: "Applications.Core/environments", Name: "env"},
			},
			extensions: []TypeSegment{
				{
					Type: "Some.Extension/extensionType", Name: "extensionResource",
				},
				{
					Type: "anotherType", Name: "",
				},
			},
			provider:      "Some.Extension",
			kind:          kindextensioncollection,
			typeName:      "Some.Extension/extensionType/anotherType",
			name:          "",
			qualifiedName: "extensionResource",
		},
		{
			id:             "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env/providers/Some.Extension/extensionType/extensionResource/anotherType/anotherName",
			expected:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env/providers/Some.Extension/extensionType/extensionResource/anotherType/anotherName",
			rootScope:      "/planes/radius/local/resourceGroups/r1",
			routingScope:   "Some.Extension/extensionType/extensionResource/anotherType/anotherName",
			parentResource: "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
			},
			types: []TypeSegment{{
				Type: "Applications.Core/environments", Name: "env"},
			},
			extensions: []TypeSegment{
				{
					Type: "Some.Extension/extensionType", Name: "extensionResource",
				},
				{
					Type: "anotherType", Name: "anotherName",
				},
			},
			provider:      "Some.Extension",
			kind:          kindextension,
			typeName:      "Some.Extension/extensionType/anotherType",
			name:          "anotherName",
			qualifiedName: "extensionResource/anotherName",
		},
		// NOTE: this is NOT actually invalid, just confusing.
		{
			id:       "/planes/radius/local/resourceGroups/r1/Applications.Core/environments/env",
			expected: "/planes/radius/local/resourceGroups/r1/Applications.Core/environments/env",
			scopes: []ScopeSegment{
				{Type: "radius", Name: "local"},
				{Type: "resourceGroups", Name: "r1"},
				{Type: "Applications.Core", Name: "environments"},
				{Type: "env", Name: ""},
			},
			provider: "",
			kind:     kindscopecollection,
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.id), func(t *testing.T) {
			t.Logf("parsing %q", v.id)
			id, err := Parse(v.id)
			require.NoError(t, err)

			require.Equalf(t, v.expected, id.id, "id comparison failed for %s", v.id)
			require.Equalf(t, v.scopes, id.scopeSegments, "scope comparison failed for %s", v.id)
			require.Equalf(t, v.types, id.typeSegments, "type comparison failed for %s", v.id)
			require.Equalf(t, v.extensions, id.extensionSegments, "extension comparison failed for %s", v.id)
			require.Equal(t, v.provider, id.ProviderNamespace(), "Provider")
			if v.rootScope != "" {
				require.Equal(t, v.rootScope, id.RootScope(), "RootScope")
			}
			if v.routingScope != "" {
				require.Equal(t, v.routingScope, id.RoutingScope(), "RoutingScope")
			}
			require.Equal(t, v.parentResource, id.ParentResource(), "ParentResource")

			if v.typeName != "" {
				require.Equal(t, v.typeName, id.Type(), "Type")
			}
			if v.name != "" {
				require.Equal(t, v.name, id.Name(), "Name")
			}
			if v.qualifiedName != "" {
				require.Equal(t, v.qualifiedName, id.QualifiedName(), "QualifiedName")
			}

			require.NotEqual(t, kindnone, v.kind, "test must specify id kind")
			require.Equal(t, v.kind == kindscope, id.IsScope(), "IsScope")
			require.Equal(t, v.kind == kindscopecollection, id.IsScopeCollection(), "IsScopeCollection")
			require.Equal(t, (v.kind == kindresource || v.kind == kindextension), id.IsResource(), "IsResource")
			require.Equal(t, (v.kind == kindresourcecollection || v.kind == kindextensioncollection), id.IsResourceCollection(), "IsResourceCollection")
			require.Equal(t, v.kind == kindextension, id.IsExtensionResource(), "IsExtensionResource")
			require.Equal(t, v.kind == kindextensioncollection, id.IsExtensionCollection(), "IsExtensionCollection")
		})
	}
}

func Test_ParseResource_Valid(t *testing.T) {
	input := "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env"
	_, err := ParseResource(input)
	require.NoError(t, err)
}

func Test_ParseResource_InvalidID(t *testing.T) {
	input := "//planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env"
	_, err := ParseResource(input)
	require.Error(t, err)
}

func Test_ParseResource_NotResource(t *testing.T) {
	input := "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env/listAction"
	_, err := ParseResource(input)
	require.Error(t, err)
}

func Test_ParseScope_Valid(t *testing.T) {
	input := "/planes/radius/local/resourceGroups/r1"
	_, err := ParseScope(input)
	require.NoError(t, err)
}

func Test_ParseScope_InvalidID(t *testing.T) {
	input := "//planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env"
	_, err := ParseScope(input)
	require.Error(t, err)
}

func Test_ParseResource_NotScope(t *testing.T) {
	input := "/planes/radius/local/resourceGroups/r1/listAction"
	_, err := ParseScope(input)
	require.Error(t, err)
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
			actual := MakeRelativeID(scopes, v.types, nil)
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
			Segment:  "subscriptions",
			Expected: "s1",
		},
		{
			ID:       "/subscriPtions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app",
			Segment:  "Subscriptions",
			Expected: "s1",
		},
		{
			ID:       "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/test-app",
			Segment:  "resourcegroups",
			Expected: "r1",
		},
		{
			ID:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			Segment:  "radius",
			Expected: "local",
		},
		{
			ID:       "/planes/radius/local/resourceGroups/r1/providers/Applications.Core/environments/env",
			Segment:  "resourcegroups",
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

func Test_Append_Extension(t *testing.T) {
	id, err := Parse("/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.Storage/storageAccounts/test-account/providers/Some.Extension/extensionType/extensionResource")
	require.NoError(t, err)

	appended := id.Append(TypeSegment{Name: "test-name", Type: "test-resource"})
	require.Equal(t, "/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/providers/Microsoft.Storage/storageAccounts/test-account/providers/Some.Extension/extensionType/extensionResource/test-resource/test-name", appended.id)
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

func Test_Truncate_Success_Extension(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.Storage/storageAccounts/test-account/providers/Some.Extension/extensionType/extensionResource/anotherType/anotherName")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.Storage/storageAccounts/test-account/providers/Some.Extension/extensionType/extensionResource", truncated.id)
}

func Test_Truncate_Success_Scope_UCP(t *testing.T) {
	id, err := Parse("/planes/azure/azurecloud/subscriptions/s1/resourceGroups/r1/")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/planes/azure/azurecloud/subscriptions/s1", truncated.id)
}

func Test_Truncate_Success_Scope_Empty(t *testing.T) {
	id, err := Parse("/planes/azure/azurecloud")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/planes", truncated.id)
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

func Test_Truncate_ReturnsSelfForTopLevelExtension(t *testing.T) {
	id, err := Parse("/subscriptions/s1/resourceGroups/r1/providers/Microsoft.Storage/storageAccounts/test-account/providers/Some.Extension/extensionType/extensionResource")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/subscriptions/s1/resourceGroups/r1/providers/Microsoft.Storage/storageAccounts/test-account/providers/Some.Extension/extensionType/extensionResource", truncated.id)
}

func Test_Truncate_WithCustomAction(t *testing.T) {
	id, err := Parse("/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0/listSecrets")
	require.NoError(t, err)

	truncated := id.Truncate()
	require.Equal(t, "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0", truncated.id)
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
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0/listSecrets",
			method: http.MethodPost,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "ucp-get",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			method: http.MethodGet,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "ucp-list",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases",
			method: http.MethodGet,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "ucp-put",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			method: http.MethodPut,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "ucp-patch",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			method: http.MethodPatch,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "ucp-delete",
			id:     "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			method: http.MethodDelete,
			err:    false,
			eID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		}, {
			desc:   "arm-post-with-custom-action",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0/listSecrets",
			method: http.MethodPost,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "arm-get",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			method: http.MethodGet,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "arm-list",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases",
			method: http.MethodGet,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "arm-put",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			method: http.MethodPut,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "arm-patch",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			method: http.MethodPatch,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
		},
		{
			desc:   "arm-delete",
			id:     "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			method: http.MethodDelete,
			err:    false,
			eID:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			eRType: ds_ctrl.MongoDatabasesResourceType,
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

func Test_Type(t *testing.T) {
	values := []struct {
		desc     string
		id       string
		expected string
	}{
		{
			desc:     "Plane scope",
			id:       "/planes",
			expected: "",
		},
		{
			desc:     "Plane resource",
			id:       "/planes/radius/local",
			expected: "System.Planes/radius",
		},
		{
			desc:     "Resourcegroup scope",
			id:       "/planes/radius/local/resourceGroups",
			expected: "",
		},
		{
			desc:     "Resourcegroup resource",
			id:       "/planes/radius/local/resourceGroups/rg1",
			expected: ResourceGroupType,
		},
		{
			desc:     "Datasource RP resource",
			id:       "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			expected: "Applications.Datastores/mongoDatabases",
		},
		{
			desc:     "AWS resource",
			id:       "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.Kinesis/Stream/stream-1",
			expected: "AWS.Kinesis/Stream",
		},
		{
			desc:     "Azure resource",
			id:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo-database-0",
			expected: "Applications.Datastores/mongoDatabases",
		},
	}
	for _, tt := range values {
		t.Run(tt.desc, func(t *testing.T) {
			rID, err := Parse(tt.id)
			require.NoError(t, err)
			require.Equal(t, tt.expected, rID.Type())
		})
	}
}

func Test_ParseProviderScope(t *testing.T) {
	values := []struct {
		desc                  string
		scope                 string
		expectedScopeSegments []ScopeSegment
	}{
		{
			desc:  "Azure provider resource group",
			scope: "/subscriptions/test-sub/resourcegroups/test-rg",
			expectedScopeSegments: []ScopeSegment{
				{Type: "subscriptions", Name: "test-sub"},
				{Type: "resourcegroups", Name: "test-rg"},
			},
		},
		{
			desc:  "AWS provider region",
			scope: "/planes/aws/aws/accounts/000/regions/us-east-1",
			expectedScopeSegments: []ScopeSegment{
				{Type: "aws", Name: "aws"},
				{Type: "accounts", Name: "000"},
				{Type: "regions", Name: "us-east-1"},
			},
		},
	}

	for _, tt := range values {
		t.Run(tt.desc, func(t *testing.T) {
			rID, err := Parse(tt.scope)
			require.NoError(t, err)
			require.Equal(t, rID.scopeSegments, tt.expectedScopeSegments)
		})
	}
}
