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

package cosmosdb

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
	"github.com/vippsas/go-cosmosdb/cosmosapi"
)

var randomSubscriptionIDs = []string{
	"eaf9116d-84e7-4720-a841-67ca2b67f888",
	"7826d962-510f-407a-92a2-5aeb37aa7b6e",
	"b2c7913e-e1fe-4c1d-a843-212159d07e46",
}
var randomResourceGroups = []string{
	"red-group",
	"blue-group",
	"radius-lala",
}
var randomPlanes = []string{
	"local",
	"k8s",
	"azure",
}

var (
	// To run this test, you need to specify the below environment variable before running the test.
	dBUrl     = os.Getenv("TEST_COSMOSDB_URL")
	masterKey = os.Getenv("TEST_COSMOSDB_MASTERKEY")

	dbName           = "applicationscore"
	dbCollectionName = "functional-test-environments"

	testLocation            = "test-location"
	environmentResourceType = "applications.core/environments"
)

func getTestEnvironmentModel(rootScope string, resourceName string) *datamodel.Environment {
	testID := rootScope + "/providers/applications.core/environments/" + resourceName

	env := &datamodel.Environment{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       testID,
				Name:     resourceName,
				Type:     environmentResourceType,
				Location: testLocation,
			},
			InternalMetadata: v1.InternalMetadata{},
		},
		Properties: datamodel.EnvironmentProperties{
			Compute: rpv1.EnvironmentCompute{
				Kind: rpv1.KubernetesComputeKind,
				KubernetesCompute: rpv1.KubernetesComputeProperties{
					ResourceID: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
					Namespace:  "default",
				},
			},
		},
	}

	env.InternalMetadata.CreatedAPIVersion = "2022-03-15-privatepreview"
	env.InternalMetadata.UpdatedAPIVersion = "2022-03-15-privatepreview"

	return env
}

var dbClient *CosmosDBStorageClient

func mustGetTestClient(t *testing.T) *CosmosDBStorageClient {
	if dBUrl == "" || masterKey == "" {
		t.Skip("TEST_COSMOSDB_URL and TEST_COSMOSDB_MASTERKEY are not set.")
	}

	if dbClient != nil {
		return dbClient
	}

	var err error
	dbClient, err = NewCosmosDBStorageClient(&ConnectionOptions{
		Url:            dBUrl,
		DatabaseName:   dbName,
		CollectionName: dbCollectionName,
		MasterKey:      masterKey,
	})

	if err != nil {
		panic(err)
	}

	if dbClient.Init(context.Background()) != nil {
		panic(err)
	}

	return dbClient
}

func TestConstructCosmosDBQuery(t *testing.T) {
	tests := []struct {
		desc        string
		storeQuery  store.Query
		queryString string
		params      []cosmosapi.QueryParam
		err         error
	}{
		{
			desc:       "invalid-query-parameters",
			storeQuery: store.Query{},
			err:        &store.ErrInvalid{Message: "RootScope can not be empty."},
		},
		{
			desc:       "scope-recursive-and-routing-scope-prefix",
			storeQuery: store.Query{RootScope: "/subscriptions/00000000-0000-0000-1000-000000000001", RoutingScopePrefix: "prefix"},
			err:        &store.ErrInvalid{Message: "RoutingScopePrefix is not supported."},
		},
		{
			desc:        "root-scope-subscription-id",
			storeQuery:  store.Query{RootScope: "/subscriptions/00000000-0000-0000-1000-000000000001", ScopeRecursive: true},
			queryString: "SELECT * FROM c WHERE STARTSWITH(c.rootScope, @rootScope, true)",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001",
			}},
			err: nil,
		},
		{
			desc:        "root-scope-plane",
			storeQuery:  store.Query{RootScope: "/planes/radius/local", ScopeRecursive: true},
			queryString: "SELECT * FROM c WHERE STARTSWITH(c.rootScope, @rootScope, true)",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/planes/radius/local",
			}},
			err: nil,
		},
		{
			desc:        "root-scope-subscription-id-and-resource-group",
			storeQuery:  store.Query{RootScope: "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup", ScopeRecursive: false},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup",
			}},
			err: nil,
		},

		{
			desc:        "root-scope-plane-and-resource-group",
			storeQuery:  store.Query{RootScope: "/planes/radius/local/resourcegroups/testgroup", ScopeRecursive: false},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/planes/radius/local/resourcegroups/testgroup",
			}},
			err: nil,
		},
		{
			storeQuery: store.Query{
				RootScope:    "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup",
				ResourceType: "applications.core/environments",
			},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope and STRINGEQUALS(c.entity.type, @rtype, true)",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup",
			}, {
				Name:  "@rtype",
				Value: "applications.core/environments",
			}},
			err: nil,
		},
		{
			storeQuery: store.Query{
				RootScope:    "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup",
				ResourceType: "applications.core/environments",
				Filters: []store.QueryFilter{
					{
						Field: "properties.environment",
						Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup/providers/applications.core/environments/env0",
					},
					{
						Field: "properties.application",
						Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup/providers/applications.core/applications/app0",
					},
				},
			},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope and STRINGEQUALS(c.entity.type, @rtype, true) and STRINGEQUALS(c.entity.properties.environment, @filter0, true) and STRINGEQUALS(c.entity.properties.application, @filter1, true)",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup",
			}, {
				Name:  "@rtype",
				Value: "applications.core/environments",
			}, {
				Name:  "@filter0",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup/providers/applications.core/environments/env0",
			}, {
				Name:  "@filter1",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testgroup/providers/applications.core/applications/app0",
			}},
			err: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			qry, err := constructCosmosDBQuery(tc.storeQuery)
			if tc.err != nil {
				require.ErrorIs(t, tc.err, err)
			} else {
				require.Equal(t, tc.queryString, qry.Query)
				require.ElementsMatch(t, tc.params, qry.Params)
			}
		})
	}
}

func TestGetNotFound(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t)

	resourceID := "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/environments/notfound"
	_, err := client.Get(ctx, resourceID)
	require.ErrorIs(t, &store.ErrNotFound{ID: resourceID}, err)
}

func TestDeleteNotFound(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t)

	resourceID := "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/environments/notfound"
	err := client.Delete(ctx, resourceID)
	require.ErrorIs(t, &store.ErrNotFound{ID: resourceID}, err)
}

func TestSave(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t)

	ucpRootScope := fmt.Sprintf("/planes/radius/local/resourcegroups/%s", randomResourceGroups[0])
	ucpResource := getTestEnvironmentModel(ucpRootScope, "test-UCP-resource")

	armResourceRootScope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s", randomSubscriptionIDs[0], randomResourceGroups[0])
	armResource := getTestEnvironmentModel(armResourceRootScope, "test-Resource")

	setupTest := func(tb testing.TB, resource *datamodel.Environment) (func(tb testing.TB), *store.Object) {
		// Prepare DB object
		obj := &store.Object{
			Metadata: store.Metadata{
				ID: resource.ID,
			},
			Data: resource,
		}

		// Save the object
		err := client.Save(ctx, obj)
		require.NoError(tb, err)
		require.NotEmpty(tb, obj.ETag)

		// Return teardown func and the object
		return func(tb testing.TB) {
			// Delete object if it exists
			err = client.Delete(ctx, resource.ID)
			require.NoError(tb, err)
		}, obj
	}

	// useObjEtag lets you use the existing object etag
	tests := map[string]struct {
		resource   *datamodel.Environment
		useObjEtag bool
		etag       string
		useOpts    bool
		err        error
	}{
		"upsert-ucp-resource-without-etag": {
			resource:   ucpResource,
			useObjEtag: false,
			etag:       "",
			useOpts:    false,
			err:        nil,
		},
		"upsert-arm-resource-without-etag": {
			resource:   armResource,
			useObjEtag: false,
			etag:       "",
			useOpts:    false,
			err:        nil,
		},
		"upsert-ucp-resource-with-valid-etag": {
			resource:   ucpResource,
			useObjEtag: true,
			etag:       "",
			useOpts:    false,
			err:        nil,
		},
		"upsert-arm-resource-with-valid-etag": {
			resource:   armResource,
			useObjEtag: true,
			etag:       "",
			useOpts:    false,
			err:        nil,
		},
		"upsert-ucp-resource-with-options": {
			resource:   ucpResource,
			useObjEtag: false,
			etag:       "",
			useOpts:    true,
			err:        nil,
		},
		"upsert-arm-resource-with-options": {
			resource:   armResource,
			useObjEtag: false,
			etag:       "",
			useOpts:    true,
			err:        nil,
		},
		"upsert-ucp-resource-with-invalid-etag": {
			resource:   ucpResource,
			useObjEtag: false,
			etag:       "invalid-etag",
			useOpts:    false,
			err:        &store.ErrConcurrency{},
		},
		"upsert-arm-resource-with-invalid-etag": {
			resource:   armResource,
			useObjEtag: false,
			etag:       "invalid-etag",
			useOpts:    false,
			err:        &store.ErrConcurrency{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			teardownTest, obj := setupTest(t, tc.resource)
			defer teardownTest(t)

			// Update the etag
			if !tc.useObjEtag {
				obj.ETag = tc.etag
			}

			// Upsert the object
			var err error
			if tc.useOpts {
				err = client.Save(ctx, obj, store.WithETag(obj.ETag))
			} else {
				err = client.Save(ctx, obj)
			}

			// Error checking
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestQuery tests the following scenarios:
// - Query records by subscription
// - Query records by plane
// - Query records by subscription and resource group
// - Query records by plane and resource group
// - Query records by subscription and resource type
// - Query records by subscription, resource group, and resource type
// - Query records by subscription, resource group, and custom filter
// - Query records by resource type and custom filter (across subscription)
//   - Use case - this will be used when environment queries all linked applications and links.
func TestQuery(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t)

	ucpResources := []string{}
	armResources := []string{}

	// TODO: UCP doesn't check for the plane type
	// Ex: /planes/radius/azure/resourcegroups/rg/.../environments/env
	// is equal to /planes/radius/local/resourcegroups/rg/.../environments/env

	setupTest := func(tb testing.TB) func(tb testing.TB) {
		// Reset arrays each time
		ucpResources = []string{}
		armResources = []string{}

		// Creates ucp resources under 3 different planes and 3 different resource groups.
		// Total makes 9 ucp resources
		for _, plane := range randomPlanes {
			for _, resourceGroup := range randomResourceGroups {
				// Create and Save a UCP Resource
				ucpRootScope := fmt.Sprintf("/planes/radius/%s/resourcegroups/%s", plane, resourceGroup)
				ucpEnv := buildAndSaveTestModel(ctx, t, ucpRootScope, uuid.New().String())
				ucpResources = append(ucpResources, ucpEnv.ID)
			}
		}

		// Creates ARM resources under 3 different subscriptions and 3 different resource groups.
		// Total makes 9 ARM resources
		for _, subscriptionID := range randomSubscriptionIDs {
			for idx, resourceGroup := range randomResourceGroups {
				// Create and Save an ARM Resource
				armResourceRootScope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s", subscriptionID, resourceGroup)
				armEnv := buildAndSaveTestModel(ctx, t, armResourceRootScope, fmt.Sprintf("test-env-%d", idx))
				armResources = append(armResources, armEnv.ID)
			}
		}

		// Return teardown
		return func(tb testing.TB) {
			// Delete all UCP resources after each test
			for i := 0; i < len(ucpResources); i++ {
				ucpResourceID := ucpResources[i]
				err := client.Delete(ctx, ucpResourceID)
				require.NoError(tb, err)
			}

			// Delete all ARM resources after each test
			for i := 0; i < len(armResources); i++ {
				armResourceID := armResources[i]
				err := client.Delete(ctx, armResourceID)
				require.NoError(tb, err)
			}
		}
	}

	queryTest := func(resourceID string, resourceType string, filters []store.QueryFilter, itemsLen int) {
		parsedID, err := resources.Parse(resourceID)
		require.NoError(t, err)

		// Build the query for testing
		query := store.Query{
			RootScope: parsedID.RootScope(),
		}
		if resourceType != "" {
			query.ResourceType = resourceType
		}
		if len(filters) > 0 {
			query.Filters = filters
		}

		results, err := client.Query(ctx, query)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.NotNil(t, results.Items)
		require.Equal(t, itemsLen, len(results.Items))
	}

	// Query with subscriptionID + resourceGroup
	tests := map[string]struct {
		resourceType string
		filters      []store.QueryFilter
		expected     int
	}{
		"just-root-scope": {
			resourceType: "",
			filters:      []store.QueryFilter{},
			expected:     1,
		},
		"root-scope-with-resource-type": {
			resourceType: environmentResourceType,
			filters:      []store.QueryFilter{},
			expected:     1,
		},
		"root-scope-resource-type-location-filter": {
			resourceType: environmentResourceType,
			filters: []store.QueryFilter{
				{
					Field: "location",
					Value: testLocation,
				},
			},
			expected: 1,
		},
		"root-scope-resource-type-wrong-location-filter": {
			resourceType: environmentResourceType,
			filters: []store.QueryFilter{
				{
					Field: "location",
					Value: "wrong-location",
				},
			},
			expected: 0,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			teardownTest := setupTest(t)
			defer teardownTest(t)

			// Query each ucp resource
			for i := 0; i < len(ucpResources); i++ {
				ucpResource := ucpResources[i]
				queryTest(ucpResource, tc.resourceType, tc.filters, tc.expected)
			}

			// Query each ARM resource
			for i := 0; i < len(armResources); i++ {
				armResource := armResources[i]
				queryTest(armResource, tc.resourceType, tc.filters, tc.expected)
			}
		})
	}

	// Query with subscriptionID or plane - These are recursive queries
	subscriptionIDCases := map[string]struct {
		rootScope    string
		resourceType string
		filters      []store.QueryFilter
		expected     int
	}{
		"arm-resource-subscription-id": {
			rootScope:    fmt.Sprintf("/subscriptions/%s", randomSubscriptionIDs[0]),
			resourceType: "",
			filters:      []store.QueryFilter{},
			expected:     3,
		},
		"ucp-resource-subscription-id": {
			rootScope:    "/planes/radius/local",
			resourceType: "",
			filters:      []store.QueryFilter{},
			expected:     3,
		},
		"arm-resource-subscription-id-with-resource-type": {
			rootScope:    fmt.Sprintf("/subscriptions/%s", randomSubscriptionIDs[0]),
			resourceType: environmentResourceType,
			filters:      []store.QueryFilter{},
			expected:     3,
		},
		"ucp-resource-subscription-id-with-resource-type": {
			rootScope:    "/planes/radius/local",
			resourceType: environmentResourceType,
			filters:      []store.QueryFilter{},
			expected:     3,
		},
		"arm-resource-subscription-id-with-resource-type-with-filter": {
			rootScope:    fmt.Sprintf("/subscriptions/%s", randomSubscriptionIDs[0]),
			resourceType: environmentResourceType,
			filters: []store.QueryFilter{
				{
					Field: "location",
					Value: testLocation,
				},
			},
			expected: 3,
		},
		"ucp-resource-subscription-id-with-resource-type-with-filter": {
			rootScope:    "/planes/radius/local",
			resourceType: environmentResourceType,
			filters: []store.QueryFilter{
				{
					Field: "location",
					Value: testLocation,
				},
			},
			expected: 3,
		},
		"arm-resource-subscription-id-with-resource-type-with-invalid-filter": {
			rootScope:    fmt.Sprintf("/subscriptions/%s", randomSubscriptionIDs[0]),
			resourceType: environmentResourceType,
			filters: []store.QueryFilter{
				{
					Field: "location",
					Value: "wrong-location",
				},
			},
			expected: 0,
		},
		"ucp-resource-subscription-id-with-resource-type-with-invalid-filter": {
			rootScope:    "/planes/radius/local",
			resourceType: environmentResourceType,
			filters: []store.QueryFilter{
				{
					Field: "location",
					Value: "wrong-location",
				},
			},
			expected: 0,
		},
	}
	for name, tc := range subscriptionIDCases {
		t.Run(name, func(t *testing.T) {
			teardownTest := setupTest(t)
			defer teardownTest(t)

			// Build the query for testing
			query := store.Query{
				RootScope:      tc.rootScope,
				ScopeRecursive: true,
			}
			if tc.resourceType != "" {
				query.ResourceType = tc.resourceType
			}
			if len(tc.filters) > 0 {
				query.Filters = tc.filters
			}

			results, err := client.Query(ctx, query)
			require.NoError(t, err)
			require.NotNil(t, results)
			require.NotNil(t, results.Items)
			require.Equal(t, tc.expected, len(results.Items))

			for _, item := range results.Items {
				if !strings.HasPrefix(item.Metadata.ID, tc.rootScope) {
					require.Failf(t, "Matched an item that doesn't include the rootscope %s", item.ID)
				}
			}
		})
	}
}

// TestPaginationTokenAndQueryItemCount tests the pagination scenario using continuation token and query item count.
func TestPaginationTokenAndQueryItemCount(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t)

	ucpResources := []string{}
	armResources := []string{}

	ucpRootScope := "/planes/radius/local/resourcegroups/test-RG"
	armResourceRootScope := "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-RG"

	setupTest := func(tb testing.TB) func(tb testing.TB) {
		// 50 UCP - 50 ARM
		for i := 0; i < 50; i++ {
			ucpEnv := buildAndSaveTestModel(ctx, t, ucpRootScope, fmt.Sprintf("ucp-env-%d", i))
			ucpResources = append(ucpResources, ucpEnv.ID)

			armEnv := buildAndSaveTestModel(ctx, t, armResourceRootScope, fmt.Sprintf("test-ENV-%d", i))
			armResources = append(armResources, armEnv.ID)
		}

		// Return teardown
		return func(tb testing.TB) {
			for i := 0; i < 50; i++ {
				ucpResourceID := ucpResources[i]
				err := client.Delete(ctx, ucpResourceID)
				require.NoError(tb, err)

				armResourceID := armResources[i]
				err = client.Delete(ctx, armResourceID)
				require.NoError(tb, err)
			}
		}
	}

	tests := map[string]struct {
		rootScope string
		itemCount string
	}{
		"ucp-resource-default-query-item-count": {
			rootScope: ucpRootScope,
			itemCount: "",
		},
		"arm-resource-default-query-item-count": {
			rootScope: armResourceRootScope,
			itemCount: "",
		},
		"ucp-resource-10-query-item-count": {
			rootScope: strings.ToLower(ucpRootScope), // case-insensitive query
			itemCount: "10",
		},
		"arm-resource-10-query-item-count": {
			rootScope: armResourceRootScope,
			itemCount: "10",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			teardownTest := setupTest(t)
			defer teardownTest(t)

			remaining := 50
			queryItemCount := defaultQueryItemCount
			paginationToken := ""

			for remaining > 0 {
				// Build query options
				queryOptions := []store.QueryOptions{}
				if tc.itemCount != "" {
					ic, err := strconv.Atoi(tc.itemCount)
					require.NoError(t, err)
					queryOptions = append(queryOptions, store.WithMaxQueryItemCount(ic))
					queryItemCount = ic
				}
				if paginationToken != "" {
					queryOptions = append(queryOptions, store.WithPaginationToken(paginationToken))
				}

				if remaining < queryItemCount {
					queryItemCount = remaining
				}

				results, err := client.Query(ctx, store.Query{RootScope: tc.rootScope}, queryOptions...)
				require.NoError(t, err)
				require.Equal(t, queryItemCount, len(results.Items))

				remaining -= queryItemCount

				if remaining > 0 {
					require.NotEmpty(t, results.PaginationToken)
					paginationToken = results.PaginationToken
				} else {
					require.Empty(t, results.PaginationToken)
				}
			}
		})
	}
}

func TestGetPartitionKey(t *testing.T) {
	cases := []struct {
		desc   string
		fullID string
		out    string
	}{
		{
			"env-partition-key",
			"/subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testGroup/providers/applications.core/environments/env0",
			"00000000000000001000000000000001",
		},
		{
			"env-no-subscription-partition-key",
			"/resourcegroups/testGroup/providers/applications.core/environments/env0",
			"",
		},
		{
			"ucp-resource-partition-key-radius-local",
			"/planes/radius/local/resourcegroups/testGroup/providers/applications.core/environments/env0",
			"RADIUSLOCAL",
		},
		{
			"ucp-resource-partition-key-radius-k8s",
			"/planes/radius/k8s/resourcegroups/testGroup/providers/applications.core/environments/env0",
			"RADIUSK8S",
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			testID, err := resources.Parse(tc.fullID)
			require.NoError(t, err)
			key, err := GetPartitionKey(testID)
			require.NoError(t, err)
			require.Equal(t, tc.out, key)
		})
	}
}

func buildAndSaveTestModel(ctx context.Context, t *testing.T, rootScope string, resourceName string) *datamodel.Environment {
	model := getTestEnvironmentModel(rootScope, resourceName)
	obj := &store.Object{
		Metadata: store.Metadata{
			ID: model.ID,
		},
		Data: model,
	}
	err := dbClient.Save(ctx, obj)
	require.NoError(t, err)
	return model
}
