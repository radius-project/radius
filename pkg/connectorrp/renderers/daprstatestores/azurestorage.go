// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func GetDaprStateStoreAzureStorage(resource datamodel.DaprStateStore, applicationName string, namespace string) (outputResources []outputresource.OutputResource, err error) {
	var azuretableStorageID resources.ID
	if resource.Properties.Kind == datamodel.DaprStateStoreKindAzureTableStorage {
		properties := resource.Properties.DaprStateStoreAzureTableStorage
		if properties.Resource == "" {
			return nil, renderers.ErrResourceMissingForResource
		}
		//Validate fully qualified resource identifier of the source resource is supplied for this connector
		azuretableStorageID, err = resources.Parse(properties.Resource)
		if err != nil {
			return []outputresource.OutputResource{}, errors.New("the 'resource' field must be a valid resource id")
		}

	}
	if resource.Properties.Kind == datamodel.DaprStateStoreKindStateSqlServer {
		properties := resource.Properties.DaprStateStoreSQLServer
		if properties.Resource == "" {
			return nil, renderers.ErrResourceMissingForResource
		}
		//Validate fully qualified resource identifier of the source resource is supplied for this connector
		azuretableStorageID, err = resources.Parse(properties.Resource)
		if err != nil {
			return []outputresource.OutputResource{}, errors.New("the 'resource' field must be a valid resource id")
		}
	}
	err = azuretableStorageID.ValidateResourceType(StorageAccountResourceType)
	if err != nil {
		return []outputresource.OutputResource{}, fmt.Errorf("the 'resource' field must refer to a Storage Table")
	}
	// generate data we can use to connect to a Storage Account
	outputResources = []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDDaprStateStoreAzureStorage,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureStorage,
				Provider: providers.ProviderAzure,
			},
			Resource: map[string]string{
				handlers.KubernetesNameKey:       resource.Name,
				handlers.KubernetesNamespaceKey:  namespace,
				handlers.ApplicationName:         applicationName,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",

				handlers.ResourceIDKey:         azuretableStorageID.String(),
				handlers.StorageAccountNameKey: azuretableStorageID.TypeSegments()[0].Name,
				handlers.ResourceName:          azuretableStorageID.TypeSegments()[2].Name,
			},
		},
	}
	return outputResources, nil
}
