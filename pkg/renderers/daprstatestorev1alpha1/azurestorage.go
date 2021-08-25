// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/workloads"
)

func GetDaprStateStoreAzureStorage(w workloads.InstantiatedWorkload, component DaprStateStoreComponent) ([]outputresource.OutputResource, error) {
	resourceKind := outputresource.KindDaprStateStoreAzureStorage
	localID := outputresource.LocalIDDaprStateStoreAzureStorage

	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, renderers.ErrResourceSpecifiedForManagedResource
		}
		resource := outputresource.OutputResource{
			LocalID: localID,
			Kind:    resourceKind,
			Type:    outputresource.TypeARM,
			Managed: true,
			Resource: map[string]string{
				handlers.ManagedKey:              "true",
				handlers.KubernetesNameKey:       w.Name,
				handlers.KubernetesNamespaceKey:  w.Application,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ComponentNameKey:        w.Name,
			},
		}

		return []outputresource.OutputResource{resource}, nil
	} else {
		if component.Config.Resource == "" {
			return nil, renderers.ErrResourceMissingForUnmanagedResource
		}
		accountID, err := renderers.ValidateResourceID(component.Config.Resource, StorageAccountResourceType, "Storage Account")
		if err != nil {
			return nil, err
		}

		// generate data we can use to connect to a Storage Account
		resource := outputresource.OutputResource{
			LocalID: localID,
			Kind:    resourceKind,
			Type:    outputresource.TypeARM,
			Managed: false,
			Resource: map[string]string{
				handlers.ManagedKey:              "false",
				handlers.KubernetesNameKey:       w.Name,
				handlers.KubernetesNamespaceKey:  w.Application,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",

				handlers.StorageAccountIDKey:   accountID.ID,
				handlers.StorageAccountNameKey: accountID.Types[0].Name,
			},
		}
		return []outputresource.OutputResource{resource}, nil
	}
}
