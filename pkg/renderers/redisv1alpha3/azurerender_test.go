// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import (
	"testing"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
)

func Test_Render_Managed_Azure_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-redis",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
		},
	}

	output, err := renderer.Render(ctx, resource, map[string]renderers.RendererDependency{})
	require.NoError(t, err)

	require.Len(t, output.Resources, 1)

	require.Equal(t, outputresource.LocalIDAzureRedis, output.Resources[0].LocalID)
	require.Equal(t, resourcekinds.AzureRedis, output.Resources[0].ResourceKind)

	expectedProperties := map[string]string{
		handlers.ManagedKey:    "true",
		handlers.RedisBaseName: "test-redis",
	}
	require.Equal(t, expectedProperties, output.Resources[0].Resource)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		// NOTE: this is NOT a secret, it doesn't contain the access keys.
		"connectionString": {
			LocalID:           outputresource.LocalIDAzureRedis,
			PropertyReference: handlers.RedisConnectionStringKey,
		},
		"host": {
			LocalID:           outputresource.LocalIDAzureRedis,
			PropertyReference: handlers.RedisHostKey,
		},
		"port": {
			LocalID:           outputresource.LocalIDAzureRedis,
			PropertyReference: handlers.RedisPortKey,
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	expectedSecretValues := map[string]renderers.SecretValueReference{
		"primaryKey": {
			LocalID:       outputresource.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
		},
		"secondaryKey": {
			LocalID:       outputresource.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/secondaryKey",
		},
	}
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Render_AzureRedis_Unmanaged_Failure(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-redis",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": false,
		},
	}

	_, err := renderer.Render(ctx, resource, map[string]renderers.RendererDependency{})
	require.Error(t, err)
	require.Equal(t, "only managed = true is support for azure redis workload", err.Error())
}
