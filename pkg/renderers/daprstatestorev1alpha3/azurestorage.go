// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha3

import (
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

func GetDaprStateStoreAzureStorage(resource renderers.RendererResource, properties Properties) ([]outputresource.OutputResource, error) {
	resourceKind := resourcekinds.DaprStateStoreAzureStorage
	localID := outputresource.LocalIDDaprStateStoreAzureStorage

	if properties.Managed {
		if properties.Resource != "" {
			return nil, renderers.ErrResourceSpecifiedForManagedResource
		}
		resource := outputresource.OutputResource{
			LocalID:      localID,
			ResourceKind: resourceKind,
			Managed:      true,
			Resource: map[string]string{
				handlers.ManagedKey:              "true",
				handlers.KubernetesNameKey:       resource.ResourceName,
				handlers.KubernetesNamespaceKey:  resource.ApplicationName,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ResourceName:            resource.ResourceName,
			},
		}

		return []outputresource.OutputResource{resource}, nil
	} else {
		if properties.Resource == "" {
			return nil, renderers.ErrResourceMissingForUnmanagedResource
		}
		accountID, err := renderers.ValidateResourceID(properties.Resource, StorageAccountResourceType, "Storage Account")
		if err != nil {
			return nil, err
		}

		// generate data we can use to connect to a Storage Account
		resource := outputresource.OutputResource{
			LocalID:      localID,
			ResourceKind: resourceKind,
			Managed:      false,
			Resource: map[string]string{
				handlers.ManagedKey:              "false",
				handlers.KubernetesNameKey:       resource.ResourceName,
				handlers.KubernetesNamespaceKey:  resource.ApplicationName,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",

				handlers.StorageAccountIDKey:   accountID.ID,
				handlers.StorageAccountNameKey: accountID.Types[0].Name,
			},
		}
		return []outputresource.OutputResource{resource}, nil
	}
}
