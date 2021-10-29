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

const (
	applicationName = "test-app"
	resourceName    = "test-redis"
)

func Test_Render_Managed_Azure_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 1)

	require.Equal(t, outputresource.LocalIDAzureRedis, output.Resources[0].LocalID)
	require.Equal(t, resourcekinds.AzureRedis, output.Resources[0].ResourceKind)
	require.Equal(t, true, output.Resources[0].Managed)

	expectedProperties := map[string]string{
		handlers.ManagedKey:    "true",
		handlers.RedisBaseName: resourceName,
	}
	require.Equal(t, expectedProperties, output.Resources[0].Resource)

	expectedComputedValues, expectedSecretValues := expectedComputedAndSecretValues()
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Azure_Render_Unmanaged_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Cache/Redis/test-redis",
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 1)

	require.Equal(t, outputresource.LocalIDAzureRedis, output.Resources[0].LocalID)
	require.Equal(t, resourcekinds.AzureRedis, output.Resources[0].ResourceKind)
	require.Equal(t, false, output.Resources[0].Managed)

	expectedProperties := map[string]string{
		handlers.ManagedKey:         "false",
		handlers.RedisResourceIdKey: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Cache/Redis/test-redis",
		handlers.RedisNameKey:       resourceName,
	}
	require.Equal(t, expectedProperties, output.Resources[0].Resource)

	expectedComputedValues, expectedSecretValues := expectedComputedAndSecretValues()
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Azure_Render_Unmanaged_MissingResource(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": false,
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, renderers.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
}

func Test_Azure_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Foo/Redis/test-redis",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a Redis Cache", err.Error())
}

func expectedComputedAndSecretValues() (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"username": {
			LocalID:           outputresource.LocalIDAzureRedis,
			PropertyReference: handlers.RedisUsernameKey,
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

	expectedSecretValues := map[string]renderers.SecretValueReference{
		"password": {
			LocalID:       outputresource.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
		},
	}

	return expectedComputedValues, expectedSecretValues
}
