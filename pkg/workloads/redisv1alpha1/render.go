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

	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

type Renderer struct {
	RedisFunc func(workloads.InstantiatedWorkload, RedisComponent) ([]workloads.OutputResource, error)
	Arm       armauth.ArmConfig
}

func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	properties := resources[0].Properties
	redisName := properties[handlers.RedisNameKey]

	rc := azclients.NewRedisClient(r.Arm.SubscriptionID, r.Arm.Auth)

	accessKeys, err := rc.ListKeys(ctx, r.Arm.ResourceGroup, redisName)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve keys: %w", err)
	}

	bindings := map[string]components.BindingState{
		"redis": {
			Component: workload.Name,
			Binding:   "redis",
			Kind:      "redislabs.com/Redis",
			Properties: map[string]interface{}{
				"connectionString": redisName + ".redis.cache.windows.net:6380",
				"host":             redisName + ".redis.cache.windows.net",
				"port":             6380, // TODO figure out if I can get these from client
				"primaryKey":       *accessKeys.PrimaryKey,
				"secondarykey":     *accessKeys.SecondaryKey,
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
		return []workloads.OutputResource{}, err
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
	if r.RedisFunc == nil {
		return []workloads.OutputResource{}, errors.New("must support either kubernetes or ARM")
	}

	return r.RedisFunc(w, component)
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
}
