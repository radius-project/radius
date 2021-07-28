// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
)

var SupportedAzureStateStoreKindValues = map[string]func(workloads.InstantiatedWorkload, DaprStateStoreComponent) ([]outputresource.OutputResource, error){
	"any":                      GetDaprStateStoreAzureStorage,
	"state.azure.tablestorage": GetDaprStateStoreAzureStorage,
	"state.sqlserver":          GetDaprStateStoreSQLServer,
}

var SupportedKubernetesStateStoreKindValues = map[string]func(workloads.InstantiatedWorkload, DaprStateStoreComponent) ([]outputresource.OutputResource, error){
	"any":         GetDaprStateStoreKubernetesRedis,
	"state.redis": GetDaprStateStoreKubernetesRedis,
}

// Renderer is the WorkloadRenderer implementation for the dapr statestore workload.
type Renderer struct {
	StateStores map[string]func(workloads.InstantiatedWorkload, DaprStateStoreComponent) ([]outputresource.OutputResource, error)
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
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := DaprStateStoreComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	if r.StateStores == nil {
		return []outputresource.OutputResource{}, errors.New("must support either kubernetes or ARM")
	}

	stateStoreFunc := r.StateStores[component.Config.Kind]
	if stateStoreFunc == nil {
		return nil, fmt.Errorf("%s is not supported. Supported kind values: %s", component.Config.Kind, getAlphabeticallySortedKeys(r.StateStores))
	}
	return stateStoreFunc(w, component)
}

func getAlphabeticallySortedKeys(store map[string]func(workloads.InstantiatedWorkload, DaprStateStoreComponent) ([]workloads.OutputResource, error)) []string {
	keys := make([]string, len(store))

	i := 0
	for k := range store {
		keys[i] = k
		i++
	}

	sort.Strings(keys)
	return keys
}
