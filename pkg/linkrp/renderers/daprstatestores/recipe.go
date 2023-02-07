// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

const (
	kubernetesAPIVersionKey = "dapr.io/v1alpha1"
	kubernetesKindKey       = "Component"
)

// Render DaprStateStore Azure recipe
func GetDaprStateStoreRecipe(resource *datamodel.DaprStateStore, applicationName string, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	err := renderers.ValidateLinkType(resource, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	recipeData := linkrp.RecipeData{
		RecipeProperties: options.RecipeProperties,
		APIVersion:       clientv2.StateStoreClientAPIVersion,
	}

	outputResources := []rpv1.OutputResource{
		{
			LocalID: rpv1.LocalIDDaprStateStoreAzureStorage,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureStorage,
				Provider: resourcemodel.ProviderAzure,
			},
			ProviderResourceType: azresources.StorageStorageAccounts,
			Resource: map[string]string{
				handlers.KubernetesNameKey:       resource.Name,
				handlers.KubernetesNamespaceKey:  options.Namespace,
				handlers.ApplicationName:         applicationName,
				handlers.KubernetesAPIVersionKey: kubernetesAPIVersionKey,
				handlers.KubernetesKindKey:       kubernetesKindKey,
				handlers.ResourceName:            resource.Name,
			},
			RadiusManaged: to.Ptr(true),
		},
		{
			LocalID: rpv1.LocalIDAzureStorageTableService,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureTableService,
				Provider: resourcemodel.ProviderAzure,
			},
			ProviderResourceType: azresources.StorageStorageAccounts + "/" + azresources.StorageStorageTableServices,
			RadiusManaged:        to.BoolPtr(false), // Deleting storage account will delete all the underlying resources
			Dependencies:         []rpv1.Dependency{{LocalID: rpv1.LocalIDDaprStateStoreAzureStorage}},
		},
		{
			LocalID: rpv1.LocalIDAzureStorageTable,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureTable,
				Provider: resourcemodel.ProviderAzure,
			},
			ProviderResourceType: azresources.StorageStorageAccounts + "/" + azresources.StorageStorageTableServices + "/" + azresources.StorageStorageAccountsTables,
			RadiusManaged:        to.BoolPtr(false), // Deleting storage account will delete all the underlying resources
			Dependencies:         []rpv1.Dependency{{LocalID: rpv1.LocalIDAzureStorageTableService}},
		},
	}

	return renderers.RendererOutput{
		Resources:            outputResources,
		RecipeData:           recipeData,
		EnvironmentProviders: options.EnvironmentProviders,
	}, nil
}
