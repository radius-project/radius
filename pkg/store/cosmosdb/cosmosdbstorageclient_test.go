// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"
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
	env.InternalMetadata.SubscriptionID = NormalizeSubscriptionID(azID.SubscriptionID)
	env.InternalMetadata.ResourceGroup = NormalizeResourceGroup(azID.ResourceGroup)
	env.InternalMetadata.CreatedAPIVersion = "2022-03-15-privatepreview"
	env.InternalMetadata.UpdatedAPIVersion = "2022-03-15-privatepreview"

	return env
}

func mustGetTestClient(dbName, collName string) *CosmosDBStorageClient {
	client, err := NewCosmosDBStorageClient(&ConnectionOptions{
		Url:            "https://radius-eastus-test.documents.azure.com:443/",
		DatabaseName:   dbName,
		CollectionName: collName,
		KeyAuth: &CosmosDBKeyAuthOptions{
			MasterKey: "fake==",
		},
	})

	if err != nil {
		panic(err)
	}

	if client.Init() != nil {
		panic(err)
	}

	return client
}

func TestSave(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient("applicationscore", "environments")
	const testResourceName = "envsavetest"
	env := getTestEnvironmentModel(fakeSubs[0], fakeResourceGroups[0], testResourceName)

	t.Run("succeeded to upsert new resource without etag", func(t *testing.T) {
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

	t.Run("succeeded to upsert new resource with same etag", func(t *testing.T) {
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

	t.Run("failed to upsert new resource with different etag", func(t *testing.T) {
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

		r.ETag = "different_tag"
		_, err = client.Save(ctx, r)
		require.Error(t, err)
		require.Equal(t, "The operation specified an eTag that is different from the version available at the server", err.Error())
	})
}

func TestQuery(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient("applicationscore", "environments")

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
			require.Equal(t, len(fakeResourceGroups), len(results))
		}
	})

	t.Run("Query all resources at resourcegroup level using RootScope", func(t *testing.T) {
		for _, id := range testIDs {
			azID, _ := azresources.Parse(id)
			rootScope := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s", azID.SubscriptionID, azID.ResourceGroup)
			results, err := client.Query(ctx, store.Query{RootScope: rootScope})
			require.NoError(t, err)
			require.NotNil(t, results)
			require.Equal(t, 1, len(results))
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
		require.Equal(t, len(fakeResourceGroups), len(results))
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
		require.Equal(t, 1, len(results))
	})

	// tear down
	for _, id := range testIDs {
		err := client.Delete(ctx, id)
		require.NoError(t, err)
	}
}

func TestContinuationToken(t *testing.T) {
	ctx := context.Background()
	client := mustGetTestClient("applicationscore", "environments")

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
	require.Equal(t, 20, len(results))
	require.NotEmpty(t, results[0].ContinuationToken)

	results, err = client.Query(ctx, store.Query{RootScope: rootScope, ContinuationToken: results[0].ContinuationToken})
	require.NoError(t, err)
	require.Equal(t, 20, len(results))
	require.NotEmpty(t, results[0].ContinuationToken)

	results, err = client.Query(ctx, store.Query{RootScope: rootScope, ContinuationToken: results[0].ContinuationToken})
	require.NoError(t, err)
	require.Equal(t, 10, len(results))
	require.Empty(t, results[0].ContinuationToken)

	// tear down
	for _, id := range testIDs {
		err := client.Delete(ctx, id)
		require.NoError(t, err)
	}
}
