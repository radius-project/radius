// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// package storetest contains SHARED testing logic that is common to our data-store implementations.
package storetest

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/etag"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

const (
	ResourceType1       = "System.Resources/resourceType1"
	ResourceType2       = "System.Resources/resourceType2"
	NestedResourceType1 = "System.Resources/resourceType1/nestedType"

	ResourcePath1       = "System.Resources/resourceType1/resource1"
	ResourcePath2       = "System.Resources/resourceType2/resource2"
	NestedResourcePath1 = "System.Resources/resourceType1/resource1/nestedType/nested1"

	RadiusScope         = "ucp:/planes/radius/local/"
	ResourceGroup1Scope = "ucp:/planes/radius/local/resourceGroups/group1"
	ResourceGroup2Scope = "ucp:/planes/radius/local/resourceGroups/group2"
	ARMResourceScope    = "/subscriptions/abc/resourceGroups/group3"
	APIVersion          = "test-api-version"
)

var ResourceGroup1ID = parseOrPanic(ResourceGroup1Scope)
var ResourceGroup2ID = parseOrPanic(ResourceGroup2Scope)
var Resource1ID = parseOrPanic(ResourceGroup1Scope + "/providers/" + ResourcePath1)
var Resource2ID = parseOrPanic(ResourceGroup2Scope + "/providers/" + ResourcePath2)
var NestedResource1ID = parseOrPanic(ResourceGroup1Scope + "/providers/" + NestedResourcePath1)
var ARMResource = parseOrPanic(ARMResourceScope + "/providers/" + ResourcePath1)

var ResourceGroup1Data = map[string]interface{}{
	"value": "1",
	"properties": map[string]interface{}{
		"group": "1",
	},
}

var ResourceGroup2Data = map[string]interface{}{
	"value": "2",
	"properties": map[string]interface{}{
		"group": "2",
	},
}

var Data1 = map[string]interface{}{
	"value": "1",
	"properties": map[string]interface{}{
		"resource": "1",
	},
}
var Data2 = map[string]interface{}{
	"value": "2",
	"properties": map[string]interface{}{
		"resource": "2",
	},
}

var NestedData1 = map[string]interface{}{
	"value": "3",
	"properties": map[string]interface{}{
		"resource": "3",
	},
}

func MarshalOrPanic(in interface{}) []byte {
	b, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}

	return b
}

func parseOrPanic(id string) resources.ID {
	parsed, err := resources.Parse(id)
	if err != nil {
		panic(err)
	}

	return parsed
}

func createObject(id resources.ID, data interface{}) store.Object {
	return store.Object{
		Metadata: store.Metadata{
			ID:          id.String(),
			APIVersion:  APIVersion,
			ContentType: "application/json",
		},
		Data: data,
	}
}

func compareObjects(t *testing.T, expected *store.Object, actual *store.Object) {
	t.Helper()

	// Compare everything except ETags
	expectedCopy := *expected
	expectedCopy.ETag = ""

	actualCopy := *actual
	actualCopy.ETag = ""

	require.Equal(t, expectedCopy, actualCopy)
}

func CompareObjectLists(t *testing.T, expected []store.Object, actual []store.Object) {
	t.Helper()

	expectedCopy := []store.Object{}
	expectedCopy = append(expectedCopy, expected...)

	actualCopy := []store.Object{}
	actualCopy = append(actualCopy, actual...)

	// Compare everything except ETags
	for i := range expectedCopy {
		expectedCopy[i].ETag = ""
	}

	for i := range actualCopy {
		actualCopy[i].ETag = ""
	}

	require.ElementsMatch(t, expectedCopy, actualCopy)
}

func RunTest(t *testing.T, client store.StorageClient, clear func(t *testing.T)) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	t.Run("get_not_found", func(t *testing.T) {
		clear(t)

		obj, err := client.Get(ctx, Resource1ID.String())
		require.ErrorIs(t, err, &store.ErrNotFound{})
		require.Nil(t, obj)
	})

	t.Run("delete_not_found", func(t *testing.T) {
		clear(t)

		err := client.Delete(ctx, Resource1ID.String())
		require.ErrorIs(t, err, &store.ErrNotFound{})
	})

	t.Run("save_and_get_arm", func(t *testing.T) {
		clear(t)
		// Testing that we can work with both UCP and ARM IDs.

		obj1 := createObject(ARMResource, Data1)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)
		require.NotEmpty(t, obj1.ETag)

		obj1Get, err := client.Get(ctx, ARMResource.String())
		require.NoError(t, err)
		compareObjects(t, &obj1, obj1Get)
		require.Equal(t, obj1Get.ETag, obj1.ETag)
	})

	t.Run("save_and_get_ucp", func(t *testing.T) {
		clear(t)
		// Testing that we can work with both UCP and ARM IDs.

		obj1 := createObject(Resource1ID, Data1)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		obj1Get, err := client.Get(ctx, Resource1ID.String())
		require.NoError(t, err)
		compareObjects(t, &obj1, obj1Get)
	})

	t.Run("save_and_get_scope", func(t *testing.T) {
		clear(t)
		// Testing that we can work with a scope like any other resource

		obj1 := createObject(ResourceGroup1ID, ResourceGroup1Data)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		obj1Get, err := client.Get(ctx, ResourceGroup1ID.String())
		require.NoError(t, err)
		compareObjects(t, &obj1, obj1Get)
	})

	t.Run("save_can_update", func(t *testing.T) {
		clear(t)

		obj1 := createObject(Resource1ID, Data1)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		obj1.Data = Data2
		err = client.Save(ctx, &obj1)
		require.NoError(t, err)

		obj1Get, err := client.Get(ctx, Resource1ID.String())
		require.NoError(t, err)
		compareObjects(t, &obj1, obj1Get)
	})

	t.Run("save_can_update_matching_etag", func(t *testing.T) {
		clear(t)

		obj1 := createObject(Resource1ID, Data1)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)
		require.NotEmpty(t, obj1.ETag)

		obj1.Data = Data2
		err = client.Save(ctx, &obj1, store.WithETag(obj1.ETag))
		require.NoError(t, err)

		obj1Get, err := client.Get(ctx, Resource1ID.String())
		require.NoError(t, err)
		compareObjects(t, &obj1, obj1Get)
	})

	t.Run("save_cannot_update_not_matching_etag", func(t *testing.T) {
		clear(t)

		obj1 := createObject(Resource1ID, Data1)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		obj1.Data = Data2
		err = client.Save(ctx, &obj1, store.WithETag(etag.New(MarshalOrPanic(Data2))))
		require.ErrorIs(t, err, &store.ErrConcurrency{})

		obj1.Data = Data1
		obj1Get, err := client.Get(ctx, Resource1ID.String())
		require.NoError(t, err)
		compareObjects(t, &obj1, obj1Get)
	})

	t.Run("save_cannot_update_missing_resource_with_not_matching_etag", func(t *testing.T) {
		clear(t)

		obj1 := createObject(Resource1ID, Data1)

		err := client.Save(ctx, &obj1, store.WithETag(etag.New(MarshalOrPanic(Data1))))
		require.ErrorIs(t, err, &store.ErrConcurrency{})

		obj1Get, err := client.Get(ctx, Resource1ID.String())
		require.ErrorIs(t, err, &store.ErrNotFound{})
		require.Nil(t, obj1Get)
	})

	t.Run("save_and_get_scope_only", func(t *testing.T) {
		clear(t)

		obj1 := createObject(parseOrPanic(ResourceGroup1Scope), Data1)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		obj1Get, err := client.Get(ctx, ResourceGroup1Scope)
		require.NoError(t, err)
		compareObjects(t, &obj1, obj1Get)
	})

	t.Run("save_and_delete", func(t *testing.T) {
		clear(t)

		obj1 := createObject(Resource1ID, Data1)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		err = client.Delete(ctx, Resource1ID.String())
		require.NoError(t, err)

		obj1Get, err := client.Get(ctx, Resource1ID.String())
		require.ErrorIs(t, err, &store.ErrNotFound{})
		require.Nil(t, obj1Get)
	})

	t.Run("save_and_delete_can_delete_with_matching_etag", func(t *testing.T) {
		clear(t)

		obj1 := createObject(Resource1ID, Data1)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		err = client.Delete(ctx, Resource1ID.String(), store.WithETag(obj1.ETag))
		require.NoError(t, err)

		obj1Get, err := client.Get(ctx, Resource1ID.String())
		require.ErrorIs(t, err, &store.ErrNotFound{})
		require.Nil(t, obj1Get)
	})

	t.Run("save_and_delete_cannot_delete_with_non_matching_etag", func(t *testing.T) {
		clear(t)

		obj1 := createObject(Resource1ID, Data1)
		err := client.Save(ctx, &obj1)
		require.NoError(t, err)

		err = client.Delete(ctx, Resource1ID.String(), store.WithETag(etag.New(MarshalOrPanic(Data2))))
		require.ErrorIs(t, err, &store.ErrConcurrency{})

		obj1Get, err := client.Get(ctx, Resource1ID.String())
		require.NoError(t, err)
		require.NotNil(t, obj1Get)
	})

	t.Run("delete_cannot_delete_missing_resource_with_not_matching_etag", func(t *testing.T) {
		clear(t)

		err := client.Delete(ctx, Resource1ID.String(), store.WithETag(etag.New(MarshalOrPanic(Data1))))
		require.ErrorIs(t, err, &store.ErrConcurrency{})
	})

	t.Run("list_can_be_empty", func(t *testing.T) {
		clear(t)

		objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope})
		require.NoError(t, err)
		require.Empty(t, objs)
	})

	t.Run("query", func(t *testing.T) {
		clear(t)

		group1 := createObject(ResourceGroup1ID, ResourceGroup1Data)
		err := client.Save(ctx, &group1)
		require.NoError(t, err)

		group2 := createObject(ResourceGroup2ID, ResourceGroup2Data)
		err = client.Save(ctx, &group2)
		require.NoError(t, err)

		obj1 := createObject(Resource1ID, Data1)
		err = client.Save(ctx, &obj1)
		require.NoError(t, err)

		nested1 := createObject(NestedResource1ID, NestedData1)
		err = client.Save(ctx, &nested1)
		require.NoError(t, err)

		obj2 := createObject(Resource2ID, Data2)
		err = client.Save(ctx, &obj2)
		require.NoError(t, err)

		t.Run("query_resources_at_resource_group_scope", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: ResourceGroup1Scope})
			require.NoError(t, err)
			expected := []store.Object{
				obj1,
				nested1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_resources_at_resource_group_scope_with_field_filter", func(t *testing.T) {
			filters := []store.QueryFilter{{Field: "value", Value: "1"}}
			objs, err := client.Query(ctx, store.Query{RootScope: ResourceGroup1Scope, Filters: filters})
			require.NoError(t, err)
			expected := []store.Object{
				obj1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_resources_at_resource_group_scope_with_prefix", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: ResourceGroup1Scope, RoutingScopePrefix: ResourcePath1})
			require.NoError(t, err)
			expected := []store.Object{
				obj1,
				nested1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_resources_at_resource_group_scope_with_type_filter", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: ResourceGroup1Scope, ResourceType: NestedResourceType1})
			require.NoError(t, err)
			expected := []store.Object{
				nested1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_resources_at_resource_group_scope_with_prefix_and_type_filter", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: ResourceGroup1Scope, RoutingScopePrefix: ResourcePath1, ResourceType: NestedResourceType1})
			require.NoError(t, err)
			expected := []store.Object{
				nested1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_scopes_at_resource_group_scope", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: ResourceGroup1Scope, IsScopeQuery: true})
			require.NoError(t, err)
			expected := []store.Object{
				group1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_scopes_at_resource_group_scope_with_type_filter", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: ResourceGroup1Scope, IsScopeQuery: true, ResourceType: "resourceGroups"})
			require.NoError(t, err)
			expected := []store.Object{
				group1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_resources_at_plane_scope", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope})
			require.NoError(t, err)
			require.Empty(t, objs)
		})

		t.Run("query_resources_at_plane_scope_recursive", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope, ScopeRecursive: true})
			require.NoError(t, err)
			expected := []store.Object{
				obj1,
				obj2,
				nested1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_resources_at_plane_scope_recursive_with_field_filter", func(t *testing.T) {
			filters := []store.QueryFilter{{Field: "value", Value: "1"}}
			objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope, ScopeRecursive: true, Filters: filters})
			require.NoError(t, err)
			expected := []store.Object{
				obj1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_resources_at_plane_scope_recursive_with_prefix", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope, ScopeRecursive: true, RoutingScopePrefix: ResourcePath1})
			require.NoError(t, err)
			expected := []store.Object{
				obj1,
				nested1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_resources_at_plane_scope_recursive_and_type_filter", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope, ScopeRecursive: true, ResourceType: NestedResourceType1})
			require.NoError(t, err)
			expected := []store.Object{
				nested1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_resources_at_plane_scope_recursive_with_prefix_and_type_filter", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope, ScopeRecursive: true, RoutingScopePrefix: ResourcePath1, ResourceType: NestedResourceType1})
			require.NoError(t, err)
			expected := []store.Object{
				nested1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_scopes_at_plane_scope_recursive", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope, ScopeRecursive: true, IsScopeQuery: true})
			require.NoError(t, err)
			expected := []store.Object{
				group1,
				group2,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_scopes_at_plane_scope_recursive_with_field_filter", func(t *testing.T) {
			filters := []store.QueryFilter{{Field: "value", Value: "1"}}
			objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope, ScopeRecursive: true, IsScopeQuery: true, Filters: filters})
			require.NoError(t, err)
			expected := []store.Object{
				group1,
			}
			CompareObjectLists(t, expected, objs.Items)
		})

		t.Run("query_scopes_at_plane_scope_recursive_with_type_filter", func(t *testing.T) {
			objs, err := client.Query(ctx, store.Query{RootScope: RadiusScope, ScopeRecursive: true, IsScopeQuery: true, ResourceType: "resourceGroups"})
			require.NoError(t, err)
			expected := []store.Object{
				group1,
				group2,
			}
			CompareObjectLists(t, expected, objs.Items)
		})
	})
}
