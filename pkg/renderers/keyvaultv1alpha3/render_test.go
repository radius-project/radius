// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha3

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

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-vault",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Equal(t, outputresource.LocalIDKeyVault, output.Resources[0].LocalID)
	require.Equal(t, resourcekinds.AzureKeyVault, output.Resources[0].ResourceKind)

	expectedProperties := map[string]string{
		handlers.ManagedKey: "true",
	}
	require.Equal(t, expectedProperties, output.Resources[0].Resource)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"uri": {
			LocalID:           outputresource.LocalIDKeyVault,
			PropertyReference: handlers.KeyVaultURIKey,
		},
	}

	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Empty(t, output.SecretValues)
}

func Test_Render_Unmanaged_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-vault",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.KeyVault/vaults/test-vault",
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Equal(t, outputresource.LocalIDKeyVault, output.Resources[0].LocalID)
	require.Equal(t, resourcekinds.AzureKeyVault, output.Resources[0].ResourceKind)

	expectedProperties := map[string]string{
		handlers.ManagedKey:      "false",
		handlers.KeyVaultIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.KeyVault/vaults/test-vault",
		handlers.KeyVaultNameKey: "test-vault",
	}
	require.Equal(t, expectedProperties, output.Resources[0].Resource)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"uri": {
			LocalID:           outputresource.LocalIDKeyVault,
			PropertyReference: handlers.KeyVaultURIKey,
		},
	}

	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Empty(t, output.SecretValues)
}

func Test_Render_Unmanaged_MissingResource(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-vault",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": false,
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, renderers.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
}

func Test_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-vault",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/vaults/test-vault",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a KeyVault", err.Error())
}
