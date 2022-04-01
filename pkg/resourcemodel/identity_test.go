// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcemodel

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

var values = []ResourceIdentity{
	{
		ResourceType: &ResourceType{
			Type:     resourcekinds.AzureCosmosAccount,
			Provider: providers.ProviderAzure,
		},
		Data: ARMIdentity{
			ID:         "/some/id",
			APIVersion: "2020-01-01",
		},
	},
	{
		ResourceType: &ResourceType{
			Type:     resourcekinds.Deployment,
			Provider: providers.ProviderKubernetes,
		},
		Data: KubernetesIdentity{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
			Name:       "test-name",
			Namespace:  "test-namespace",
		},
	},
	{
		ResourceType: &ResourceType{
			Type:     resourcekinds.AzurePodIdentity,
			Provider: providers.ProviderAzureKubernetesService,
		},
		Data: AADPodIdentityIdentity{
			AKSClusterName: "test-cluster",
			Name:           "test-name",
			Namespace:      "test-namespace",
		},
	},
}

// Test that all formats of ResourceIdentifier round-trip with BSON
func Test_ResourceIdentifier_BSONRoundTrip(t *testing.T) {
	for _, input := range values {
		t.Run(string(input.ResourceType.Type), func(t *testing.T) {
			b, err := bson.Marshal(&input)
			require.NoError(t, err)

			output := ResourceIdentity{}
			err = bson.Unmarshal(b, &output)
			require.NoError(t, err)

			require.Equal(t, input, output)
		})
	}
}

// Test that all formats of ResourceIdentifier round-trip with JSON
func Test_ResourceIdentifier_JSONRoundTrip(t *testing.T) {
	for _, input := range values {
		t.Run(string(input.ResourceType.Type), func(t *testing.T) {
			b, err := json.Marshal(&input)
			require.NoError(t, err)

			output := ResourceIdentity{}
			err = json.Unmarshal(b, &output)
			require.NoError(t, err)

			require.Equal(t, input, output)
		})
	}
}
