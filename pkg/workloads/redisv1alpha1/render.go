// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package redisv1alpha1

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

type Renderer struct {
	Redis map[string]func(workloads.InstantiatedWorkload, RedisComponent) ([]workloads.OutputResource, error)
}

// TODO how do we decide names here.
var SupportedAzureRedisKindValues = map[string]func(workloads.InstantiatedWorkload, RedisComponent) ([]workloads.OutputResource, error){
	"any":         GetAzureRedis,
	"redis":       GetAzureRedis,
	"azure.redis": GetAzureRedis,
}

var SupportedKubernetesRedisKindValues = map[string]func(workloads.InstantiatedWorkload, RedisComponent) ([]workloads.OutputResource, error){
	"any":              GetKubernetesRedis,
	"redis":            GetKubernetesRedis,
	"kubernetes.redis": GetKubernetesRedis,
}

func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	properties := resources[0].Properties
	redisName := properties[handlers.ServiceBusQueueNameKey]

	bindings := map[string]components.BindingState{}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for redis workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.OutputResource, error) {
	component := RedisComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.OutputResource{}, err
	}

<<<<<<< HEAD
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
=======
	if r.Redis == nil {
		return []workloads.OutputResource{}, errors.New("must support either kubernetes or ARM")
	}

	redisFunc := r.Redis[component.Config.Kind]
	if redisFunc == nil {
		return nil, fmt.Errorf("%s is not supported. Supported kind values: %s", component.Config.Kind, getAlphabeticallySortedKeys(r.Redis))
	}

	return redisFunc(w, component)
}

func getAlphabeticallySortedKeys(store map[string]func(workloads.InstantiatedWorkload, RedisComponent) ([]workloads.OutputResource, error)) []string {
	keys := make([]string, len(store))

	i := 0
	for k := range store {
		keys[i] = k
		i++
	}

	sort.Strings(keys)
	return keys
>>>>>>> 7d0bece (plugging things in)
}
