// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

var supportedStateStoreKindValues = [3]string{"any", "state.azure.tablestorage", "state.sqlserver"}

// Renderer is the WorkloadRenderer implementation for the dapr statestore workload.
type Renderer struct {
}

// Allocate is the WorkloadRenderer implementation for dapr statestore workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	bindings := map[string]components.BindingState{
		"default": {
			Component: workload.Name,
			Binding:   "default",
			Properties: map[string]interface{}{
				"stateStoreName": workload.Name,
			},
		},
	}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for dapr statestore workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.OutputResource, error) {
	component := DaprStateStoreComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.OutputResource{}, err
	}

	resourceKind := ""
	localID := ""
	if component.Config.Kind == "any" || component.Config.Kind == "state.azure.tablestorage" {
		resourceKind = workloads.ResourceKindDaprStateStoreAzureStorage
		localID = "DaprStateStoreAzureStorage"
	} else if component.Config.Kind == "state.sqlserver" {
		resourceKind = workloads.ResourceKindDaprStateStoreSQLServer
		localID = "DaprStateStoreSQLServer"
	} else {
		return []workloads.OutputResource{}, fmt.Errorf("%s is not supported. Supported kind values: %s", component.Config.Kind, supportedStateStoreKindValues)
	}

	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, workloads.ErrResourceSpecifiedForManagedResource
		}

		resource := workloads.OutputResource{
			LocalID:            localID,
			ResourceKind:       resourceKind,
			OutputResourceType: workloads.OutputResourceTypeArm,
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
		if component.Config.Kind == "state.sqlserver" {
			return nil, errors.New("only Radius managed resources are supported for Dapr SQL Server")
		}

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
			ResourceKind:       workloads.ResourceKindDaprStateStoreAzureStorage,
			OutputResourceType: workloads.OutputResourceTypeArm,
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
