// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
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

var (
	// To run this test, you need to specify the below environment variable before running the test.
	// TODO: Create an issue to change CI/CD pipeline
	dBUrl     = os.Getenv("TEST_COSMOSDB_URL")
	masterKey = os.Getenv("TEST_COSMOSDB_MASTERKEY")

	testLocation     = "test-location"
	dbName           = "applicationscore"
	dbCollectionName = "functional-test-environments"

	environmentResourceType = "applications.core/environments"
)

func getTestEnvironmentModel(rootScope string, resourceName string) *datamodel.Environment {
	testID := rootScope + "/providers/applications.core/environments/" + resourceName

	env := &datamodel.Environment{
		TrackedResource: v1.TrackedResource{
			ID:       testID,
			Name:     resourceName,
			Type:     environmentResourceType,
			Location: testLocation,
		},
		Properties: datamodel.EnvironmentProperties{
			Compute: datamodel.EnvironmentCompute{
				Kind:       datamodel.KubernetesComputeKind,
				ResourceID: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
			},
		},
		InternalMetadata: v1.InternalMetadata{},
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

	// Singleton
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
			err:        &store.ErrInvalid{Message: "invalid Query parameters"},
		},
		{
			desc:       "scope-recursive-and-routing-scope-prefix",
			storeQuery: store.Query{RootScope: "/subscriptions/00000000-0000-0000-1000-000000000001", RoutingScopePrefix: "prefix"},
			err:        &store.ErrInvalid{Message: "ScopeRecursive and RoutingScopePrefix are not supported."},
		},
		{
			desc:        "root-scope-subscription-id",
			storeQuery:  store.Query{RootScope: "/subscriptions/00000000-0000-0000-1000-000000000001"},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001",
			}},
			err: nil,
		},
		{
			desc:        "root-scope-subscription-id-and-resource-group",
			storeQuery:  store.Query{RootScope: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup"},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup",
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

	_, err := client.Get(ctx, "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/environments/notfound")
	require.ErrorIs(t, &store.ErrNotFound{}, err)
}

func TestDeleteNotFound(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t)

	err := client.Delete(ctx, "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/environments/notfound")
	require.ErrorIs(t, &store.ErrNotFound{}, err)
}

func TestSave(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t)

	ucpRootScope := fmt.Sprintf("ucp:/planes/radius/local/resourcegroups/%s", randomResourceGroups[0])
	ucpResource := getTestEnvironmentModel(ucpRootScope, "test-ucp-resource")

	regularResourceRootScope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s", randomSubscriptionIDs[0], randomResourceGroups[0])
	regularResource := getTestEnvironmentModel(regularResourceRootScope, "test-resource")

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
		err        bool
	}{
		"upsert-ucp-resource-without-etag": {
			resource:   ucpResource,
			useObjEtag: false,
			etag:       "",
			useOpts:    false,
			err:        false,
		},
		"upsert-regular-resource-without-etag": {
			resource:   regularResource,
			useObjEtag: false,
			etag:       "",
			useOpts:    false,
			err:        false,
		},
		"upsert-ucp-resource-with-valid-etag": {
			resource:   ucpResource,
			useObjEtag: true,
			etag:       "",
			useOpts:    false,
			err:        false,
		},
		"upsert-regular-resource-with-valid-etag": {
			resource:   regularResource,
			useObjEtag: true,
			etag:       "",
			useOpts:    false,
			err:        false,
		},
		"upsert-ucp-resource-with-options": {
			resource:   ucpResource,
			useObjEtag: false,
			etag:       "",
			useOpts:    true,
			err:        false,
		},
		"upsert-regular-resource-with-options": {
			resource:   regularResource,
			useObjEtag: false,
			etag:       "",
			useOpts:    true,
			err:        false,
		},
		"upsert-ucp-resource-with-invalid-etag": {
			resource:   ucpResource,
			useObjEtag: false,
			etag:       "invalid-etag",
			useOpts:    false,
			err:        true,
		},
		"upsert-regular-resource-with-invalid-etag": {
			resource:   regularResource,
			useObjEtag: false,
			etag:       "invalid-etag",
			useOpts:    false,
			err:        true,
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
			if tc.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestQuery tests the following scenarios:
// 1. Query records by subscription
// 2. Query records by subscription and resource group
// 3. Query records by subscription and resource type
// 4. Query records by subscription, resource group, and resource type
// 5. Query records by subscription, resource group, and custom filter
// 6. Query records by resource type and custom filter (across subscription)
//   - Use case - this will be used when environment queries all linked applications and connectors.
func TestQuery(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t)

	length := len(randomResourceGroups)
	ucpResources := []string{}
	regularResources := []string{}

	for _, randomSubscriptionID := range randomSubscriptionIDs {
		for _, randomResourceGroup := range randomResourceGroups {
			// Create and Save a UCP Resource
			ucpRootScope := fmt.Sprintf("ucp:/planes/radius/local/resourcegroups/%s",
				randomResourceGroup)
			ucpEnv := buildAndSaveTestModel(ctx, t, ucpRootScope, "ucp-env")
			ucpResources = append(ucpResources, ucpEnv.ID)

			// Create and Save a Regular/Non-UCP Resource
			regularResourceRootScope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s",
				randomSubscriptionID, randomResourceGroup)
			regularEnv := buildAndSaveTestModel(ctx, t, regularResourceRootScope, "test-env")
			regularResources = append(regularResources, regularEnv.ID)
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
			for idx := 0; idx < length; idx++ {
				ucpResource := ucpResources[idx]
				queryTest(ucpResource, tc.resourceType, tc.filters, tc.expected)

				regularResource := regularResources[idx]
				queryTest(regularResource, tc.resourceType, tc.filters, tc.expected)
			}
		})
	}

	// We have to delete everything here

	// TODO: Are we going to be able to query all resources with just a subscription id?
	// Because if the resource was saved by the RootScope, then it will not be available
	// by querying with subscription id?

	// t.Run("Query all resources at subscription level using RootScope", func(t *testing.T) {
	// 	for _, id := range testIDs {
	// 		parsedID, _ := resources.Parse(id)
	// 		rootScope := fmt.Sprintf("/subscriptions/%s", parsedID.FindScope(resources.SubscriptionsSegment))
	// 		results, err := client.Query(ctx, store.Query{RootScope: rootScope})
	// 		require.NoError(t, err)
	// 		require.NotNil(t, results)
	// 		require.NotNil(t, results.Items)
	// 		require.Equal(t, len(fakeResourceGroups), len(results.Items))
	// 	}
	// })

	// t.Run("Query all resources at subscription level and at type using RootScope, ResourceType.", func(t *testing.T) {
	// 	azID, _ := azresources.Parse(testIDs[0])
	// 	query := store.Query{
	// 		RootScope:    fmt.Sprintf("/subscriptions/%s", azID.SubscriptionID),
	// 		ResourceType: "Applications.Core/environments",
	// 	}

	// 	results, err := client.Query(ctx, query)
	// 	require.NoError(t, err)
	// 	require.NotNil(t, results)
	// 	require.NotNil(t, results.Items)
	// 	require.Equal(t, len(fakeResourceGroups), len(results.Items))
	// })

	// t.Run("Query all resources at resourcegroup level and at type using RootScope, ResourceType with filter.", func(t *testing.T) {
	// 	query := store.Query{
	// 		RootScope:    "/",
	// 		ResourceType: "applications.core/environments",
	// 	}

	// 	results, err := client.Query(ctx, query)
	// 	require.NoError(t, err)
	// 	require.NotNil(t, results)
	// 	require.NotNil(t, results.Items)
	// 	require.Equal(t, 9, len(results.Items))
	// })
}

// TestPaginationTokenAndQueryItemCount tests the pagination scenario using continuation token and query item count.
func TestPaginationTokenAndQueryItemCount(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t)

	ucpResources := []string{}
	regularResources := []string{}

	ucpRootScope := "ucp:/planes/radius/local/resourcegroups/test-rg"
	regularResourceRootScope := "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-rg"

	setupTest := func(tb testing.TB) func(tb testing.TB) {
		// 50 UCP - 50 Regular
		for i := 0; i < 50; i++ {
			ucpEnv := buildAndSaveTestModel(ctx, t, ucpRootScope, fmt.Sprintf("ucp-env-%d", i))
			ucpResources = append(ucpResources, ucpEnv.ID)

			regularEnv := buildAndSaveTestModel(ctx, t, regularResourceRootScope, fmt.Sprintf("test-env-%d", i))
			regularResources = append(regularResources, regularEnv.ID)
		}

		// Return teardown
		return func(tb testing.TB) {
			for i := 0; i < 50; i++ {
				ucpResourceID := ucpResources[i]
				err := client.Delete(ctx, ucpResourceID)
				require.NoError(tb, err)

				regularResourceID := regularResources[i]
				err = client.Delete(ctx, regularResourceID)
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
		"regular-resource-default-query-item-count": {
			rootScope: regularResourceRootScope,
			itemCount: "",
		},
		"ucp-resource-10-query-item-count": {
			rootScope: ucpRootScope,
			itemCount: "10",
		},
		"regular-resource-10-query-item-count": {
			rootScope: regularResourceRootScope,
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
			"subscriptions/00000000-0000-0000-1000-000000000001/resourcegroups/testGroup/providers/applications.core/environments/env0",
			"00000000000000001000000000000001",
		},
		{
			"env-no-subscription-partition-key",
			"resourcegroups/testGroup/providers/applications.core/environments/env0",
			"",
		},
		{
			"ucp-resource-partition-key",
			"ucp:/planes/radius/local/resourcegroups/testGroup/providers/applications.core/environments/env0",
			"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			testID, err := resources.Parse(tc.fullID)
			require.NoError(t, err)
			key := GetPartitionKey(testID)
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
