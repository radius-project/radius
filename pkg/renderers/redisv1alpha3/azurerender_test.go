// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import (
	"testing"

	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
)

const (
	applicationName = "test-app"
	resourceName    = "test-redis"
)

func Test_Azure_Render_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Cache/Redis/test-redis",
			"host": "localhost",
			"port": 42,
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 1)

	require.Equal(t, outputresource.LocalIDAzureRedis, output.Resources[0].LocalID)
	require.Equal(t, resourcekinds.AzureRedis, output.Resources[0].ResourceKind)

	expectedProperties := map[string]string{
		handlers.RedisResourceIdKey: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Cache/Redis/test-redis",
		handlers.RedisNameKey:       resourceName,
	}
	require.Equal(t, expectedProperties, output.Resources[0].Resource)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"host": {
			Value: "localhost",
		},
		"port": {
			Value: "42",
		},
		"username": {
			LocalID: "AzureRedis",
			PropertyReference: "redisusername",
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, "/primaryKey", output.SecretValues[PasswordValue].ValueSelector)
	require.Equal(t, "listKeys", output.SecretValues[PasswordValue].Action)
}

func Test_Azure_Render_User_Secrets(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"host": "localhost",
			"port": 42,
			"secrets": map[string]string{
				"password":         "deadbeef",
				"connectionString": "admin:deadbeef@localhost:42",
			},
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 0)

	expectedComputedValues, expectedSecretValues := expectedComputedAndSecretValues()
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Azure_Render_MissingResource(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition:      map[string]interface{}{},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, renderers.ErrResourceMissingForResource.Error(), err.Error())
}

func Test_Azure_Render_InvalidResourceType(t *testing.T) {
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
		"host": {
			Value: "localhost",
		},
		"port": {
			Value: "42",
		},
		"username": {
			LocalID: "AzureRedis",
			PropertyReference: "redisusername",
		},
	}

	expectedSecretValues := map[string]renderers.SecretValueReference{
		"password": {
			Value: "deadbeef",
		},
		"connectionString": {
			Value: "admin:deadbeef@localhost:42",
		},
	}

	return expectedComputedValues, expectedSecretValues
}
