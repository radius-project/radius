// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func GetDaprStateStoreAzureStorage(resource datamodel.DaprStateStore, applicationName string, namespace string) (outputResources []outputresource.OutputResource, err error) {
	var azuretableStorageID resources.ID
	if resource.Properties.Kind == datamodel.DaprStateStoreKindAzureTableStorage {
		properties := resource.Properties
		if properties.Resource == "" {
			return nil, conv.NewClientErrInvalidRequest(renderers.ErrResourceMissingForResource.Error())
		}
		//Validate fully qualified resource identifier of the source resource is supplied for this link
		azuretableStorageID, err = resources.ParseResource(properties.Resource)
		if err != nil {
			return []outputresource.OutputResource{}, conv.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
		}

	}
	if resource.Properties.Kind == datamodel.DaprStateStoreKindStateSqlServer {
		properties := resource.Properties
		if properties.Resource == "" {
			return nil, conv.NewClientErrInvalidRequest(renderers.ErrResourceMissingForResource.Error())
		}
		//Validate fully qualified resource identifier of the source resource is supplied for this link
		azuretableStorageID, err = resources.ParseResource(properties.Resource)
		if err != nil {
			return []outputresource.OutputResource{}, conv.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
		}
	}
	err = azuretableStorageID.ValidateResourceType(StorageAccountResourceType)
	if err != nil {
		return []outputresource.OutputResource{}, conv.NewClientErrInvalidRequest("the 'resource' field must refer to a Storage Table")
	}
	// generate data we can use to connect to a Storage Account
	outputResources = []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDDaprStateStoreAzureStorage,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureStorage,
				Provider: resourcemodel.ProviderAzure,
			},
			Resource: map[string]string{
				handlers.KubernetesNameKey:       resource.Name,
				handlers.KubernetesNamespaceKey:  namespace,
				handlers.ApplicationName:         applicationName,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",

				handlers.ResourceIDKey:         azuretableStorageID.String(),
				handlers.StorageAccountNameKey: azuretableStorageID.TypeSegments()[0].Name,
				handlers.ResourceName:          resource.Name,
			},
			RadiusManaged: to.BoolPtr(true),
		},
	}
	return outputResources, nil
}
