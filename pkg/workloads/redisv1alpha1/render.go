// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package redisv1alpha1

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

type Renderer struct {
	// Arm armauth.ArmConfig
}

func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	properties := resources[0].Properties
	redisName := properties[handlers.ServiceBusQueueNameKey]

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

// Render is the WorkloadRenderer implementation for redis workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.OutputResource, error) {
	component := RedisComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return nil, err
	}

	if component.Config.Managed {
		resource := workloads.OutputResource{
			LocalID:            workloads.LocalIDAzureRedis,
			ResourceKind:       workloads.KindAzureRedis,
			OutputResourceType: workloads.OutputResourceTypeArm,
			Managed:            true,
			Resource: map[string]string{
				handlers.ManagedKey:        "true",
				handlers.AzureRedisNameKey: component.Config.Name,
			},
		}
		return []workloads.OutputResource{resource}, nil
	} else {
		// TODO
	}

	return []workloads.OutputResource{}, nil
}
