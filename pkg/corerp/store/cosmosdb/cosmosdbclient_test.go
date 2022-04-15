//go:build cosmosdbtest
// +build cosmosdbtest

// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestSave(t *testing.T) {
	client, err := NewCosmosDBClient(&CosmosDBClientOptions{
		Url:                "https://radius-eastus-test.documents.azure.com:443/",
		MasterKeyAuthCreds: "fake",
		DatabaseName:       "applicationscore",
		CollectionName:     "environments",
	})

	err = client.Init()
	require.NoError(t, err)

	ctx := context.Background()

	env := &datamodel.Environment{
		TrackedResource: datamodel.TrackedResource{
			ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
			Name: "env0",
			Type: "Applications.Core/environments",
		},
		Properties: datamodel.EnvironmentProperties{
			Compute: datamodel.EnvironmentCompute{
				Kind:       datamodel.KubernetesComputeKind,
				ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
			},
		},
		InternalMetadata: datamodel.InternalMetadata{
			TenantID:       "10000000-0000-0000-0000-000000000000",
			SubscriptionID: "00000000-0000-0000-0000-000000000000",
			ResourceGroup:  "testGroup",
		},
	}
	r := &Object{
		Metadata: Metadata{
			ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
		},
		Data: env,
	}

	err = client.Save(ctx, r)
	require.NoError(t, err)

	obj, err = client.Get(ctx, r.ID)
	require.NotNil(t, obj)
	require.NoError(t, err)
}
