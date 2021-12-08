// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha3

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
)

func GetAzureFileShareVolume(ctx context.Context, arm armauth.ArmConfig, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := radclient.AzureFileShareVolumeProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	storageAccountDependency = outputresource.Dependency{
		LocalID: outputresource.LocalIDAzureFileShareStorageAccount,
	}

	resources := []outputresource.OutputResource{}
	if properties.Managed != nil && *properties.Managed {
		results, err := RenderManaged(resource.ResourceName, properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		resources = append(resources, results...)
	} else {
		results, err := RenderUnmanaged(resource.ResourceName, properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		resources = append(resources, results...)
	}

	computedValues, secretValues := MakeSecretsAndValuesForAzureFileShare(storageAccountDependency.LocalID)

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func RenderManaged(name string, properties radclient.AzureFileShareVolumeProperties) ([]outputresource.OutputResource, error) {
	if properties.Resource != nil && *properties.Resource != "" {
		return nil, renderers.ErrResourceSpecifiedForManagedResource
	}

	storageAccountResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureFileShareStorageAccount,
		ResourceKind: resourcekinds.AzureFileShareStorageAccount,
		Managed:      true,
		Resource: map[string]string{
			handlers.ManagedKey:                           "true",
			handlers.AzureFileShareStorageAccountBaseName: "azurestorageaccount",
		},
	}

	fileshareResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureFileShare,
		ResourceKind: resourcekinds.AzureFileShare,
		Managed:      true,
		Resource: map[string]string{
			handlers.ManagedKey:       "true",
			handlers.FileShareNameKey: name,
		},
		Dependencies: []outputresource.Dependency{storageAccountDependency},
	}

	return []outputresource.OutputResource{storageAccountResource, fileshareResource}, nil
}

func RenderUnmanaged(name string, properties radclient.AzureFileShareVolumeProperties) ([]outputresource.OutputResource, error) {
	if properties.Resource == nil || *properties.Resource == "" {
		return nil, renderers.ErrResourceMissingForUnmanagedResource
	}

	fileshareID, err := renderers.ValidateResourceID(*properties.Resource, AzureFileShareResourceType, "Azure File Share")
	if err != nil {
		return nil, err
	}

	// Truncate the fileservices/shares part of the ID to make an ID for the account
	storageAccountID := fileshareID.Truncate().Truncate()

	storageAccountResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureFileShareStorageAccount,
		ResourceKind: resourcekinds.AzureFileShareStorageAccount,
		Managed:      false,
		Resource: map[string]string{
			handlers.ManagedKey:                     "false",
			handlers.FileShareStorageAccountIDKey:   storageAccountID.ID,
			handlers.FileShareStorageAccountNameKey: storageAccountID.Types[0].Name,
		},
	}

	fileshareResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureFileShare,
		ResourceKind: resourcekinds.AzureFileShare,
		Managed:      false,
		Resource: map[string]string{
			handlers.ManagedKey:                     "false",
			handlers.FileShareStorageAccountIDKey:   storageAccountID.ID,
			handlers.FileShareStorageAccountNameKey: storageAccountID.Types[0].Name,
			handlers.FileShareIDKey:                 fileshareID.ID,
			handlers.FileShareNameKey:               fileshareID.Types[2].Name,
		},

		Dependencies: []outputresource.Dependency{storageAccountDependency},
		Identity:     resourcemodel.NewARMIdentity(fileshareID.ID, clients.GetAPIVersionFromUserAgent(storage.UserAgent())),
	}
	return []outputresource.OutputResource{storageAccountResource, fileshareResource}, nil
}

// MakeSecretsAndValuesForAzureFileShare returns secrets and computed values for Azure File Share
func MakeSecretsAndValuesForAzureFileShare(name string) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	computedValues := map[string]renderers.ComputedValueReference{
		StorageAccountName: {
			LocalID: outputresource.LocalIDAzureFileShareStorageAccount,
			Value:   name,
		},
	}
	secretValues := map[string]renderers.SecretValueReference{
		StorageKeyValue: {
			LocalID: storageAccountDependency.LocalID,
			// https://docs.microsoft.com/en-us/rest/api/storagerp/storage-accounts/list-keys
			Action:        "listKeys",
			ValueSelector: "/keys/0/value",
		},
	}

	return computedValues, secretValues
}
