// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"github.com/Azure/go-autorest/autorest/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func GetDaprStateStoreAzureStorage(resource *datamodel.DaprStateStore, applicationName string, namespace string) (outputResources []rpv1.OutputResource, err error) {
	properties := resource.Properties
	if properties.Resource == "" {
		return nil, v1.NewClientErrInvalidRequest(renderers.ErrResourceMissingForResource.Error())
	}
	var azuretableStorageID resources.ID
	azuretableStorageID, err = resources.ParseResource(properties.Resource)
	if err != nil {
		return []rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
	}
	err = azuretableStorageID.ValidateResourceType(StorageAccountResourceType)
	if err != nil {
		return []rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("the 'resource' field must refer to a Storage Table")
	}
	// generate data we can use to connect to a Storage Account
	outputResources = []rpv1.OutputResource{
		{
			LocalID: rpv1.LocalIDDaprStateStoreAzureStorage,
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
