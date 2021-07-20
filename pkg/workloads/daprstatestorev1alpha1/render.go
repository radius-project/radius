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
	"github.com/Azure/radius/pkg/workloads"
)

var supportedAzureStateStoreKindValues = map[string]func(workloads.InstantiatedWorkload, DaprStateStoreComponent) ([]workloads.OutputResource, error){
	"any":                      GetDaprStateStoreAzureStorage,
	"state.azure.tablestorage": GetDaprStateStoreAzureStorage,
	"state.sqlserver":          GetDaprStateStoreSQLServer,
}
var supportedAzureStoreKindValues = [3]string{"any", "state.azure.tablestorage", "state.sqlserver"}

var supportedKubernetesStateStoreKindValues = map[string]func(workloads.InstantiatedWorkload, DaprStateStoreComponent) ([]workloads.OutputResource, error){
	"any":         GetDaprStateStoreKubernetesRedis,
	"state.redis": GetDaprStateStoreKubernetesRedis,
}
var supportedKubernetesStoreKindValues = [2]string{"any", "state.redis"}

// Renderer is the WorkloadRenderer implementation for the dapr statestore workload.
type Renderer struct {
	SupportsArm        bool
	SupportsKubernetes bool
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

	if r.SupportsArm {
		stateStoreFunc := supportedAzureStateStoreKindValues[component.Config.Kind]
		if stateStoreFunc == nil {
			return nil, fmt.Errorf("%s is not supported for azure. Supported kind values: %s", component.Config.Kind, supportedAzureStoreKindValues)
		}
		return stateStoreFunc(w, component)
	} else if r.SupportsKubernetes {
		stateStoreFunc := supportedKubernetesStateStoreKindValues[component.Config.Kind]
		if stateStoreFunc == nil {
			return nil, fmt.Errorf("%s is not supported for kubernetes. Supported kind values: %s", component.Config.Kind, supportedKubernetesStoreKindValues)
		}

		return supportedKubernetesStateStoreKindValues[component.Config.Kind](w, component)
	}

	return []workloads.OutputResource{}, errors.New("must support either kubernetes or ARM")
}
