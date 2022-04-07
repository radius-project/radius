// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
)

const (
	applicationName = "test-app"
	resourceName    = "test-redis"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Cache/Redis/test-redis",
			"host":     "localhost",
			"port":     42,
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 1)

	require.Equal(t, outputresource.LocalIDAzureRedis, output.Resources[0].LocalID)
	require.Equal(t, resourcekinds.AzureRedis, output.Resources[0].ResourceType.Type)

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
			LocalID:           "AzureRedis",
			PropertyReference: "redisusername",
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, "/primaryKey", output.SecretValues[renderers.PasswordStringHolder].ValueSelector)
	require.Equal(t, "listKeys", output.SecretValues[renderers.PasswordStringHolder].Action)
}

func Test_Render_User_Secrets(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"host": "localhost",
			"port": 42,
			"secrets": map[string]string{
				renderers.PasswordStringHolder:  "deadbeef",
				renderers.ConnectionStringValue: "admin:deadbeef@localhost:42",
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

func Test_Render_NoResourceSpecified(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition:      map[string]interface{}{},
	}

	rendererOutput, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)
	require.Equal(t, 0, len(rendererOutput.Resources))
}

func Test_Render_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

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
		renderers.UsernameStringValue: {
			LocalID:           outputresource.LocalIDAzureRedis,
			PropertyReference: handlers.RedisUsernameKey,
		},
	}

	expectedSecretValues := map[string]renderers.SecretValueReference{
		renderers.PasswordStringHolder: {
			Value: "deadbeef",
		},
		renderers.ConnectionStringValue: {
			Value: "admin:deadbeef@localhost:42",
		},
	}

	return expectedComputedValues, expectedSecretValues
}
