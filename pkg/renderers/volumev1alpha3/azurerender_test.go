// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha3

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
	resourceName    = "test-db"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_Unmanaged_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{
		VolumeRenderers: map[string]RendererType{
			PersistentVolumeKindAzureFileShare: GetAzureFileShareVolume,
		},
	}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/fileservices/default/shares/test-share",
			"kind":     PersistentVolumeKindAzureFileShare,
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 2)
	accountResource := output.Resources[0]
	fileshareResource := output.Resources[1]

	require.Equal(t, outputresource.LocalIDAzureFileShareStorageAccount, accountResource.LocalID)
	require.Equal(t, resourcekinds.AzureFileShareStorageAccount, accountResource.ResourceKind)

	require.Equal(t, outputresource.LocalIDAzureFileShare, fileshareResource.LocalID)
	require.Equal(t, resourcekinds.AzureFileShare, fileshareResource.ResourceKind)

	expectedAccount := map[string]string{
		handlers.FileShareStorageAccountIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
		handlers.FileShareStorageAccountNameKey: "test-account",
	}
	require.Equal(t, expectedAccount, accountResource.Resource)

	expectedFileShare := map[string]string{
		handlers.FileShareStorageAccountIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
		handlers.FileShareStorageAccountNameKey: "test-account",
		handlers.FileShareIDKey:                 "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/fileservices/default/shares/test-share",
		handlers.FileShareNameKey:               "test-share",
	}
	require.Equal(t, expectedFileShare, fileshareResource.Resource)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		StorageAccountName: {
			LocalID: outputresource.LocalIDAzureFileShareStorageAccount,
			Value:   "AzureFileShareStorageAccount",
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	expectedSecretValues := map[string]renderers.SecretValueReference{
		StorageKeyValue: {
			LocalID:       storageAccountDependency.LocalID,
			Action:        "listKeys",
			ValueSelector: "/keys/0/value",
			Transformer:   "",
		},
	}
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Render_Unmanaged_MissingResource(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{
		VolumeRenderers: map[string]RendererType{
			PersistentVolumeKindAzureFileShare: GetAzureFileShareVolume,
		},
	}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind": PersistentVolumeKindAzureFileShare,
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, renderers.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
}

func Test_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{
		VolumeRenderers: map[string]RendererType{
			PersistentVolumeKindAzureFileShare: GetAzureFileShareVolume,
		},
	}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/storageAccounts/fileshares/test-share",
			"kind":     PersistentVolumeKindAzureFileShare,
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a Azure File Share", err.Error())
}
