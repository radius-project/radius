// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"fmt"

	"github.com/Azure/go-autorest/autorest/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

func GetDaprStateStoreRecipe(resource datamodel.DaprStateStore, applicationName string, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	if options.RecipeProperties.LinkType != resource.ResourceTypeName() {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("link type %q of provided recipe %q is incompatible with %q resource type. Recipe link type must match link resource type.",
			options.RecipeProperties.LinkType, options.RecipeProperties.Name, ResourceType))
	}

	recipeData := datamodel.RecipeData{
		Provider:         resourcemodel.ProviderAzure,
		RecipeProperties: options.RecipeProperties,
		APIVersion:       clientv2.AccountsClientAPIVersion,
	}

	outputResources := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDDaprStateStoreAzureStorage,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureStorage,
				Provider: resourcemodel.ProviderAzure,
			},
			ProviderResourceType: azresources.StorageStorageAccounts,
			Resource: map[string]string{
				handlers.KubernetesNameKey:       resource.Name,
				handlers.KubernetesNamespaceKey:  options.Namespace,
				handlers.ApplicationName:         applicationName,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ResourceName:            resource.Name,
			},
			RadiusManaged: to.BoolPtr(true),
		},
	}

	return renderers.RendererOutput{
		Resources:            outputResources,
		RecipeData:           recipeData,
		EnvironmentProviders: options.EnvironmentProviders,
	}, nil
}
