// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

func GetDaprStateStoreAzureStorage(w workloads.InstantiatedWorkload, component DaprStateStoreComponent) ([]workloads.OutputResource, error) {
	resourceKind := workloads.ResourceKindDaprStateStoreAzureStorage
	localID := workloads.LocalIDDaprStateStoreAzureStorage

	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, workloads.ErrResourceSpecifiedForManagedResource
		}
		resource := workloads.OutputResource{
			LocalID:            localID,
			ResourceKind:       resourceKind,
			OutputResourceType: workloads.OutputResourceTypeArm,
			Managed:            true,
			Resource: map[string]string{
				handlers.ManagedKey:              "true",
				handlers.KubernetesNameKey:       w.Name,
				handlers.KubernetesNamespaceKey:  w.Application,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ComponentNameKey:        w.Name,
			},
		}

		return []workloads.OutputResource{resource}, nil
	} else {
		if component.Config.Resource == "" {
			return nil, workloads.ErrResourceMissingForUnmanagedResource
		}
		accountID, err := workloads.ValidateResourceID(component.Config.Resource, StorageAccountResourceType, "Storage Account")
		if err != nil {
			return nil, err
		}

		// generate data we can use to connect to a Storage Account
		resource := workloads.OutputResource{
			LocalID:            localID,
			ResourceKind:       resourceKind,
			OutputResourceType: workloads.OutputResourceTypeArm,
			Managed:            false,
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
		return []workloads.OutputResource{resource}, nil
	}
}
