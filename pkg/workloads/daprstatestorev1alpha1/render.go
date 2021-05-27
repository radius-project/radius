// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/curp/handlers"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/workloads"
)

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
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := DaprStateStoreComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if component.Config.Kind != "any" && component.Config.Kind != "state.azure.tablestorage" {
		return []workloads.WorkloadResource{}, errors.New("only kind 'any' and 'state.azure.tablestorage' is supported right now")
	}

	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, errors.New("the 'resource' field cannot be specified when 'managed=true'")
		}

		// generate data we can use to manage a Storage Account
		resource := workloads.WorkloadResource{
			Type: workloads.ResourceKindDaprStateStoreAzureStorage,
			Resource: map[string]string{
				handlers.ManagedKey:                "true",
				handlers.KubernetesNameKey:         w.Name,
				handlers.KubernetesNamespaceKey:    w.Application,
				handlers.KubernetesAPIVersionKey:   "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:         "Component",
				handlers.StorageAccountBaseNameKey: w.Name,
			},
		}

		return []workloads.WorkloadResource{resource}, nil
	} else {
		if component.Config.Resource == "" {
			return nil, errors.New("the 'resource' field is required when 'managed' is not specified")
		}

		topicID, err := radresources.Parse(component.Config.Resource)
		if err != nil {
			return nil, errors.New("the 'resource' field must be a valid resource id.")
		}

		err = topicID.ValidateResourceType(StorageAccountResourceType)
		if err != nil {
			return nil, fmt.Errorf("the 'resource' field must refer to a Storage Account")
		}

		// generate data we can use to connect to a Storage Account
		resource := workloads.WorkloadResource{
			Type: workloads.ResourceKindDaprStateStoreAzureStorage,
			Resource: map[string]string{
				handlers.ManagedKey:              "false",
				handlers.KubernetesNameKey:       w.Name,
				handlers.KubernetesNamespaceKey:  w.Application,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",

				handlers.StorageAccountIDKey:   topicID.ID,
				handlers.StorageAccountNameKey: topicID.Types[0].Name,
			},
		}
		return []workloads.WorkloadResource{resource}, nil
	}
}
