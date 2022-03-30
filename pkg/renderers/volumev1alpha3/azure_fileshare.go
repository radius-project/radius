// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha3

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

func GetAzureFileShareVolume(ctx context.Context, arm *armauth.ArmConfig, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := radclient.AzureFileShareVolumeProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	storageAccountDependency = outputresource.Dependency{
		LocalID: outputresource.LocalIDAzureFileShareStorageAccount,
	}

	resources := []outputresource.OutputResource{}

	results, err := RenderResource(resource.ResourceName, properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resources = append(resources, results...)

	computedValues, secretValues := MakeSecretsAndValuesForAzureFileShare(storageAccountDependency.LocalID)

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func RenderResource(name string, properties radclient.AzureFileShareVolumeProperties) ([]outputresource.OutputResource, error) {
	if properties.Resource == nil || *properties.Resource == "" {
		return nil, renderers.ErrResourceMissingForResource
	}

	fileshareID, err := renderers.ValidateResourceID(*properties.Resource, AzureFileShareResourceType, "Azure File Share")
	if err != nil {
		return nil, err
	}

	// Truncate the fileservices/shares part of the ID to make an ID for the account
	storageAccountID := fileshareID.Truncate().Truncate()

	storageAccountResource := outputresource.OutputResource{
		LocalID: outputresource.LocalIDAzureFileShareStorageAccount,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureFileShareStorageAccount,
			Provider: providers.ProviderAzure,
		},
		Resource: map[string]string{
			handlers.FileShareStorageAccountIDKey:   storageAccountID.ID,
			handlers.FileShareStorageAccountNameKey: storageAccountID.Types[0].Name,
		},
	}

	fileshareResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureFileShare,
		Provider: providers.ProviderAzure,
	}
	fileshareResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureFileShare,
		ResourceType: fileshareResourceType,
		Resource: map[string]string{
			handlers.FileShareStorageAccountIDKey:   storageAccountID.ID,
			handlers.FileShareStorageAccountNameKey: storageAccountID.Types[0].Name,
			handlers.FileShareIDKey:                 fileshareID.ID,
			handlers.FileShareNameKey:               fileshareID.Types[2].Name,
		},

		Dependencies: []outputresource.Dependency{storageAccountDependency},
		Identity:     resourcemodel.NewARMIdentity(&fileshareResourceType, fileshareID.ID, clients.GetAPIVersionFromUserAgent(storage.UserAgent())),
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
