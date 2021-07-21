// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"errors"

	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

func GetDaprStateStoreSQLServer(w workloads.InstantiatedWorkload, component DaprStateStoreComponent) ([]workloads.OutputResource, error) {
	if !component.Config.Managed {
		return nil, errors.New("only Radius managed resources are supported for Dapr SQL Server")
	}
	if component.Config.Resource != "" {
		return nil, workloads.ErrResourceSpecifiedForManagedResource
	}
	// generate data we can use to connect to a Storage Account
	resource := workloads.OutputResource{
		LocalID:            workloads.LocalIDDaprStateStoreSQLServer,
		ResourceKind:       workloads.ResourceKindDaprStateStoreSQLServer,
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
}
