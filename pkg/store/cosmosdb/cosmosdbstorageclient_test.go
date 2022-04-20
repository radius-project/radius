// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"
	"github.com/vippsas/go-cosmosdb/cosmosapi"
)

var fakeSubs = []string{
	"eaf9116d-84e7-4720-a841-67ca2b67f888",
	"7826d962-510f-407a-92a2-5aeb37aa7b6e",
	"b2c7913e-e1fe-4c1d-a843-212159d07e46",
}
var fakeResourceGroups = []string{
	"red-group",
	"blue-group",
	"radius-lala",
}

var (
	// To run this test, you need to specify the below environment variable before running the test.
	dBUrl     = os.Getenv("TEST_COSMOSDB_URL")
	masterKey = os.Getenv("TEST_COSMOSDB_MASTERKEY")
)

func getRandomItem(items []string) string {
	return items[rand.Intn(len(items))]
}

func getTestEnvironmentModel(subID, rgName, resourceName string) *datamodel.Environment {
	testID := "/subscriptions/" + subID + "/resourceGroups/" + rgName + "/providers/Applications.Core/environments/" + resourceName
	env := &datamodel.Environment{
		TrackedResource: datamodel.TrackedResource{
			ID:       testID,
			Name:     resourceName,
			Type:     "Applications.Core/environments",
			Location: "WEST US",
		},
		Properties: datamodel.EnvironmentProperties{
			Compute: datamodel.EnvironmentCompute{
				Kind:       datamodel.KubernetesComputeKind,
				ResourceID: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
			},
		},
		InternalMetadata: datamodel.InternalMetadata{},
	}

	azID, _ := azresources.Parse(env.ID)
	env.InternalMetadata.SubscriptionID = strings.ToLower(azID.SubscriptionID)
	env.InternalMetadata.ResourceGroup = strings.ToLower(azID.ResourceGroup)
	env.InternalMetadata.CreatedAPIVersion = "2022-03-15-privatepreview"
	env.InternalMetadata.UpdatedAPIVersion = "2022-03-15-privatepreview"

	return env
}

func mustGetTestClient(t *testing.T, dbName, collName string) *CosmosDBStorageClient {
	if dBUrl == "" || masterKey == "" {
		t.Skip("TEST_COSMOSDB_URL and TEST_COSMOSDB_MASTERKEY are not set.")
	}

	client, err := NewCosmosDBStorageClient(&ConnectionOptions{
		Url:            dBUrl,
		DatabaseName:   dbName,
		CollectionName: collName,
		MasterKey:      masterKey,
	})

	if err != nil {
		panic(err)
	}

	if client.Init(context.Background()) != nil {
		panic(err)
	}

	return client
}

func TestConstructCosmosDBQuery(t *testing.T) {
	tests := []struct {
		storeQuery  store.Query
		queryString string
		params      []cosmosapi.QueryParam
		err         error
	}{
		{
			storeQuery: store.Query{},
			err:        &store.ErrInvalid{Message: "invalid Query parameters"},
		},
		{
			storeQuery: store.Query{RootScope: "/subscriptions/00000000-0000-0000-1000-000000000001", RoutingScopePrefix: "prefix"},
			err:        &store.ErrInvalid{Message: "ScopeRecursive and RoutingScopePrefix are not supported."},
		},
		{
			storeQuery:  store.Query{RootScope: "/subscriptions/00000000-0000-0000-1000-000000000001"},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001",
			}},
			err: nil,
		},
		{
			storeQuery:  store.Query{RootScope: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup"},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope and c.resourceGroup = @rgName",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001",
			}, {
				Name:  "@rgName",
				Value: "testgroup",
			}},
			err: nil,
		},
		{
			storeQuery: store.Query{
				RootScope:    "/subscriptions/00000000-A000-0000-1000-000000000001/resourceGroups/testGroup",
				ResourceType: "applications.core/environments",
			},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope and c.resourceGroup = @rgName and STRINGEQUALS(c.entity.type, @rtype, true)",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-a000-0000-1000-000000000001",
			}, {
				Name:  "@rgName",
				Value: "testgroup",
			}, {
				Name:  "@rtype",
				Value: "applications.core/environments",
			}},
			err: nil,
		},
		{
			storeQuery: store.Query{
				RootScope:    "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup",
				ResourceType: "applications.core/environments",
				Filters: []store.QueryFilter{
					{
						Field: "properties.environment",
						Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/environments/env0",
					},
					{
						Field: "properties.application",
						Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/applications/app0",
					},
				},
			},
			queryString: "SELECT * FROM c WHERE c.rootScope = @rootScope and c.resourceGroup = @rgName and STRINGEQUALS(c.entity.type, @rtype, true) and STRINGEQUALS(c.entity.properties.environment, @filter0, true) and STRINGEQUALS(c.entity.properties.application, @filter1, true)",
			params: []cosmosapi.QueryParam{{
				Name:  "@rootScope",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001",
			}, {
				Name:  "@rgName",
				Value: "testgroup",
			}, {
				Name:  "@rtype",
				Value: "applications.core/environments",
			}, {
				Name:  "@filter0",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/environments/env0",
			}, {
				Name:  "@filter1",
				Value: "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/applications/app0",
			}},
			err: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.queryString, func(t *testing.T) {
			_, qry, err := constructCosmosDBQuery(tc.storeQuery)
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
	client := mustGetTestClient(t, "applicationscore", "environments")

	_, err := client.Get(ctx, "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/environments/notfound")
	require.ErrorIs(t, &store.ErrNotFound{}, err)
}

func TestDeleteNotFound(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t, "applicationscore", "environments")

	err := client.Delete(ctx, "/subscriptions/00000000-0000-0000-1000-000000000001/resourceGroups/testGroup/providers/applications.core/environments/notfound")
	require.ErrorIs(t, &store.ErrNotFound{}, err)
}

func TestSave(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t, "applicationscore", "environments")
	const testResourceName = "envsavetest"
	env := getTestEnvironmentModel(fakeSubs[0], fakeResourceGroups[0], testResourceName)

	t.Run("succeeded to upsert new resource without ETag", func(t *testing.T) {
		_ = client.Delete(ctx, env.ID)
		r := &store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		obj, err := client.Save(ctx, r)
		require.NoError(t, err)
		require.NotEmpty(t, obj.ETag)

		r.ETag = ""
		_, err = client.Save(ctx, r)
		require.NoError(t, err)
	})

	t.Run("succeeded to upsert new resource with valid ETag", func(t *testing.T) {
		_ = client.Delete(ctx, env.ID)
		r := &store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		obj, err := client.Save(ctx, r)
		require.NoError(t, err)
		require.NotEmpty(t, obj.ETag)

		_, err = client.Save(ctx, r)
		require.NoError(t, err)
	})

	t.Run("succeeded to upsert new resource by WithETag", func(t *testing.T) {
		_ = client.Delete(ctx, env.ID)
		r := &store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		obj, err := client.Save(ctx, r)
		require.NoError(t, err)
		require.NotEmpty(t, obj.ETag)

		validEtag := obj.ETag
		obj.ETag = ""

		_, err = client.Save(ctx, r, store.WithETag(validEtag))
		require.NoError(t, err)
	})

	t.Run("failed to upsert new resource with invalid ETag", func(t *testing.T) {
		_ = client.Delete(ctx, env.ID)
		r := &store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		obj, err := client.Save(ctx, r)
		require.NoError(t, err)
		require.NotEmpty(t, obj.ETag)

		r.ETag = "invalid_etag"
		_, err = client.Save(ctx, r)
		require.ErrorIs(t, &store.ErrConflict{Message: "ETag is not matched."}, err)
	})
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
	client := mustGetTestClient(t, "applicationscore", "environments")

	// set up
	testIDs := []string{}

	const testResourceName = "envsavetest"
	for _, subID := range fakeSubs {
		for _, rg := range fakeResourceGroups {
			env := getTestEnvironmentModel(subID, rg, testResourceName)
			r := &store.Object{
				Metadata: store.Metadata{
					ID: env.ID,
				},
				Data: env,
			}
			_, err := client.Save(ctx, r)
			require.NoError(t, err)
			testIDs = append(testIDs, env.ID)
		}
	}

	t.Run("Query all resources at subscription level using RootScope", func(t *testing.T) {
		for _, id := range testIDs {
			azID, _ := azresources.Parse(id)
			rootScope := fmt.Sprintf("/subscriptions/%s", azID.SubscriptionID)
			results, err := client.Query(ctx, store.Query{RootScope: rootScope})
			require.NoError(t, err)
			require.NotNil(t, results)
			require.NotNil(t, results.Items)
			require.Equal(t, len(fakeResourceGroups), len(results.Items))
		}
	})

	t.Run("Query all resources at resourcegroup level using RootScope", func(t *testing.T) {
		for _, id := range testIDs {
			azID, _ := azresources.Parse(id)
			rootScope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s", azID.SubscriptionID, azID.ResourceGroup)
			results, err := client.Query(ctx, store.Query{RootScope: rootScope})
			require.NoError(t, err)
			require.NotNil(t, results.Items)
			require.Equal(t, 1, len(results.Items))
		}
	})

	t.Run("Query all resources at subscription level and at type using RootScope, ResourceType.", func(t *testing.T) {
		azID, _ := azresources.Parse(testIDs[0])
		query := store.Query{
			RootScope:    fmt.Sprintf("/subscriptions/%s", azID.SubscriptionID),
			ResourceType: "Applications.Core/environments",
		}

		results, err := client.Query(ctx, query)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.NotNil(t, results.Items)
		require.Equal(t, len(fakeResourceGroups), len(results.Items))
	})

	t.Run("Query all resources at resourcegroup level and at type using RootScope, ResourceType.", func(t *testing.T) {
		azID, _ := azresources.Parse(testIDs[0])
		query := store.Query{
			RootScope:    fmt.Sprintf("/subscriptions/%s/resourcegroups/%s", azID.SubscriptionID, azID.ResourceGroup),
			ResourceType: "applications.core/environments",
		}

		results, err := client.Query(ctx, query)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.NotNil(t, results.Items)
		require.Equal(t, 1, len(results.Items))
	})

	t.Run("Query all resources at resourcegroup level and at type using RootScope, ResourceType with filter.", func(t *testing.T) {
		azID, _ := azresources.Parse(testIDs[0])
		query := store.Query{
			RootScope:    fmt.Sprintf("/subscriptions/%s/resourcegroups/%s", azID.SubscriptionID, azID.ResourceGroup),
			ResourceType: "applications.core/environments",
			Filters: []store.QueryFilter{
				{
					Field: "location",
					Value: "WEST US",
				},
			},
		}

		results, err := client.Query(ctx, query)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.NotNil(t, results.Items)
		require.Equal(t, 1, len(results.Items))
	})

	t.Run("Query all resources at resourcegroup level and at type using RootScope, ResourceType with filter.", func(t *testing.T) {
		query := store.Query{
			RootScope:    "/",
			ResourceType: "applications.core/environments",
		}

		results, err := client.Query(ctx, query)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.NotNil(t, results.Items)
		require.Equal(t, 9, len(results.Items))
	})

	// tear down
	for _, id := range testIDs {
		err := client.Delete(ctx, id)
		require.NoError(t, err)
	}
}

// TestPaginationContinuationToken tests the pagination scenario using continuation token.
func TestPaginationContinuationToken(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient(t, "applicationscore", "environments")

	testIDs := []string{}
	// set up
	const testResourceName = "envsavetest"
	for i := 0; i < 50; i++ {
		env := getTestEnvironmentModel(fakeSubs[0], fakeResourceGroups[0], fmt.Sprintf("%s-%05d", testResourceName, i))
		r := &store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		_, err := client.Save(ctx, r)
		require.NoError(t, err)
		testIDs = append(testIDs, env.ID)
	}

	azID, _ := azresources.Parse(testIDs[0])
	rootScope := fmt.Sprintf("/subscriptions/%s", azID.SubscriptionID)

	results, err := client.Query(ctx, store.Query{RootScope: rootScope})
	require.NoError(t, err)
	require.Equal(t, 20, len(results.Items))
	require.NotEmpty(t, results.PaginationToken)

	results, err = client.Query(ctx, store.Query{RootScope: rootScope}, store.WithPaginationToken(results.PaginationToken))
	require.NoError(t, err)
	require.Equal(t, 20, len(results.Items))
	require.NotEmpty(t, results.PaginationToken)

	results, err = client.Query(ctx, store.Query{RootScope: rootScope}, store.WithPaginationToken(results.PaginationToken))
	require.NoError(t, err)
	require.Equal(t, 10, len(results.Items))
	require.Empty(t, results.PaginationToken)

	// tear down
	for _, id := range testIDs {
		err := client.Delete(ctx, id)
		require.NoError(t, err)
	}
}
