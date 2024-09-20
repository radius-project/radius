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

package storeutil

import (
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func Test_ExtractStorageParts(t *testing.T) {
	type testcase struct {
		ID           string
		Prefix       string
		RootScope    string
		RoutingScope string
		ResourceType string
	}

	cases := []testcase{
		{
			ID:           "/", // Not a valid case, just testing that we don't panic.
			Prefix:       ScopePrefix,
			RootScope:    "/",
			RoutingScope: "/",
			ResourceType: "",
		},
		{
			ID:           "/planes", // Not a valid case, just testing that we don't panic.
			Prefix:       ScopePrefix,
			RootScope:    "/planes/",
			RoutingScope: "/",
			ResourceType: "",
		},
		{
			ID:           "/planes/radius/local",
			Prefix:       ScopePrefix,
			RootScope:    "/planes/",
			RoutingScope: "/radius/local/",
			ResourceType: "radius",
		},
		{
			ID:           "/planes/radius/local/resourceGroups/cool-group",
			Prefix:       ScopePrefix,
			RootScope:    "/planes/radius/local/",
			RoutingScope: "/resourcegroups/cool-group/",
			ResourceType: "resourcegroups",
		},
		{
			ID:           "/subscriptions/cool-sub/resourceGroups/cool-group/",
			Prefix:       ScopePrefix,
			RootScope:    "/subscriptions/cool-sub/",
			RoutingScope: "/resourcegroups/cool-group/",
			ResourceType: "resourcegroups",
		},
		{
			ID:           "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			Prefix:       ResourcePrefix,
			RootScope:    "/planes/radius/local/resourcegroups/cool-group/",
			RoutingScope: "/applications.core/applications/cool-app/",
			ResourceType: "applications.core/applications",
		},
		{
			ID:           "/subscriptions/cool-sub/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app/nested/cool-nested",
			Prefix:       ResourcePrefix,
			RootScope:    "/subscriptions/cool-sub/resourcegroups/cool-group/",
			RoutingScope: "/applications.core/applications/cool-app/nested/cool-nested/",
			ResourceType: "applications.core/applications/nested",
		},
		{
			ID:           "/subscriptions/cool-sub/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			Prefix:       ResourcePrefix,
			RootScope:    "/subscriptions/cool-sub/resourcegroups/cool-group/",
			RoutingScope: "/applications.core/applications/cool-app/",
			ResourceType: "applications.core/applications",
		},
	}

	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			id, err := resources.Parse(tc.ID)
			require.NoError(t, err)

			prefix, rootScope, routingScope, resourceType := ExtractStorageParts(id)
			require.Equal(t, tc.Prefix, prefix)
			require.Equal(t, tc.RootScope, rootScope)
			require.Equal(t, tc.RoutingScope, routingScope)
			require.Equal(t, tc.ResourceType, resourceType)
		})
	}
}

func Test_IDMatchesQuery(t *testing.T) {
	type testcase struct {
		ID      string
		Query   store.Query
		IsMatch bool
	}

	cases := []testcase{
		// Tests in this section target block-coverage of all of our negative cases.
		{
			ID: "/planes/radius/local",
			Query: store.Query{
				IsScopeQuery: false, // mismatched query type
			},
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			Query: store.Query{
				IsScopeQuery: true, // mismatched query type
			},
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group",
			Query: store.Query{
				RootScope:    "/planes/radius/local/resourceGroups/cool-group", // mismatched root scope
				IsScopeQuery: true,
			},
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group",
			Query: store.Query{
				RootScope:      "/planes/radius/another-plane", // mismatched root scope
				ScopeRecursive: true,
				IsScopeQuery:   true,
			},
		},
		{
			ID: "/subscriptions/cool-sub/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app/nested/cool-nested",
			Query: store.Query{
				RootScope:          "/subscriptions/cool-sub/resourceGroups/cool-group",
				RoutingScopePrefix: "Applications.Core/applications/different-app", // mismatched routing scope prefix
				IsScopeQuery:       false,
			},
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			Query: store.Query{
				RootScope:    "/planes/radius/local/resourceGroups",
				ResourceType: "Applications.Core/containers", // mismatched resource type
				IsScopeQuery: false,
			},
		},

		// Tests in this section target our main use-cases for the query logic.
		{
			ID: "/planes/radius/local",
			Query: store.Query{
				RootScope:    "/planes", // list all planes (regardless of type)
				IsScopeQuery: true,
			},
			IsMatch: true,
		},
		{
			ID: "/planes/radius/local",
			Query: store.Query{
				RootScope:    "/planes", // list all planes (specific type)
				ResourceType: "radius",
				IsScopeQuery: true,
			},
			IsMatch: true,
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group",
			Query: store.Query{
				RootScope:    "/planes/radius/local/", // list all resource groups
				ResourceType: "resourceGroups",
				IsScopeQuery: true,
			},
			IsMatch: true,
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group",
			Query: store.Query{
				RootScope:      "/planes", // list all resource groups across planes
				ResourceType:   "resourceGroups",
				ScopeRecursive: true,
				IsScopeQuery:   true,
			},
			IsMatch: true,
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			Query: store.Query{
				RootScope:      "/planes/radius/local", // list all resources in plane
				ScopeRecursive: true,
				IsScopeQuery:   false,
			},
			IsMatch: true,
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			Query: store.Query{
				RootScope:      "/planes/radius/local/resourceGroups/cool-group", // list all resources in resource group
				ScopeRecursive: false,
				IsScopeQuery:   false,
			},
			IsMatch: true,
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			Query: store.Query{
				RootScope:      "/planes/radius/local/resourceGroups/cool-group", // list all applications in resource group
				ResourceType:   "Applications.Core/applications",
				ScopeRecursive: false,
				IsScopeQuery:   false,
			},
			IsMatch: true,
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app",
			Query: store.Query{
				RootScope:      "/planes/radius/local/", // list all applications in plane
				ResourceType:   "Applications.Core/applications",
				ScopeRecursive: true,
				IsScopeQuery:   false,
			},
			IsMatch: true,
		},
		{
			ID: "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/applications/cool-app/nested/cool-nested",
			Query: store.Query{
				RootScope:          "/planes/radius/local/resourceGroups/cool-group", // list nested resources
				RoutingScopePrefix: "/Applications.Core/applications/cool-app",
				ResourceType:       "Applications.Core/applications/nested",
				ScopeRecursive:     false,
				IsScopeQuery:       false,
			},
			IsMatch: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			id, err := resources.Parse(tc.ID)
			require.NoError(t, err)

			isMatch := IDMatchesQuery(id, tc.Query)
			require.Equal(t, tc.IsMatch, isMatch)
		})
	}
}

func Test_NormalizePart(t *testing.T) {
	type testcase struct {
		Input    string
		Expected string
	}

	cases := []testcase{
		{
			Input:    "",
			Expected: "",
		},
		{
			Input:    "part",
			Expected: "/part/",
		},
		{
			Input:    "/part",
			Expected: "/part/",
		},
		{
			Input:    "part/",
			Expected: "/part/",
		},
		{
			Input:    "/part/",
			Expected: "/part/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Input, func(t *testing.T) {
			result := NormalizePart(tc.Input)
			require.Equal(t, tc.Expected, result)
		})
	}
}

func Test_NormalizeResourceID(t *testing.T) {
	type testcase struct {
		Input    string
		Expected string
		IsError  bool
	}

	cases := []testcase{
		{
			Input:    "/planes/radius/local/resourceGroups/my-rg/providers/Applications.Core/applications/my-app",
			Expected: "/planes/radius/local/resourceGroups/my-rg/providers/Applications.Core/applications/my-app",
			IsError:  false,
		},
		{
			Input:    "/planes/radius/local/resourceGroups/my-rg",
			Expected: "/planes/radius/local/providers/System.Resources/resourceGroups/my-rg",
			IsError:  false,
		},
		{
			Input:    "/planes/azure/my-plane",
			Expected: "/planes/providers/System.Azure/planes/my-plane",
			IsError:  false,
		},
		{
			Input:    "/planes/aws/my-plane",
			Expected: "/planes/providers/System.AWS/planes/my-plane",
			IsError:  false,
		},
		{
			Input:    "/planes/radius/my-plane",
			Expected: "/planes/providers/System.Radius/planes/my-plane",
			IsError:  false,
		},
		{
			Input:    "/planes/radius/my-plane/resourceGroups/my-rg",
			Expected: "/planes/radius/my-plane/providers/System.Resources/resourceGroups/my-rg",
			IsError:  false,
		},
		{
			Input:   "/invalid/id",
			IsError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Input, func(t *testing.T) {
			id, err := resources.Parse(tc.Input)
			require.NoError(t, err)

			result, err := NormalizeResourceID(id)
			if tc.IsError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.Expected, result.String())
			}
		})
	}
}

func Test_NormalizeResourceType(t *testing.T) {
	type testcase struct {
		Input    string
		Expected string
		IsError  bool
	}

	cases := []testcase{
		{
			Input:    "Applications.Core/applications",
			Expected: "Applications.Core/applications",
			IsError:  false,
		},
		{
			Input:    "resourceGroups",
			Expected: "System.Resources/resourceGroups",
			IsError:  false,
		},
		{
			Input:    "aws",
			Expected: "System.Aws/planes",
			IsError:  false,
		},
		{
			Input:    "azure",
			Expected: "System.Azure/planes",
			IsError:  false,
		},
		{
			Input:    "radius",
			Expected: "System.Radius/planes",
			IsError:  false,
		},
		{
			Input:   "invalidType",
			IsError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Input, func(t *testing.T) {
			result, err := NormalizeResourceType(tc.Input)
			if tc.IsError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.Expected, result)
			}
		})
	}
}
