// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package redisv1alpha1

import (
	"context"
	"sort"

	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
)

type AzureRenderer struct {
	Arm armauth.ArmConfig
}

type KubernetesRenderer struct {
}

func (r AzureRenderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	return AllocateAzureBindings(r.Arm, ctx, workload, resources)
}

// Render is the WorkloadRenderer implementation for redis workload.
func (r AzureRenderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := RedisComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	return GetAzureRedis(w, component)
}

func (r KubernetesRenderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	return AllocateKubernetesBindings(ctx, workload, resources)
}

// Render is the WorkloadRenderer implementation for redis workload.
func (r KubernetesRenderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := RedisComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	return GetKubernetesRedis(w, component)
}

func getAlphabeticallySortedKeys(store map[string]func(workloads.InstantiatedWorkload, RedisComponent) ([]outputresource.OutputResource, error)) []string {
	keys := make([]string, len(store))

	i := 0
	for k := range store {
		keys[i] = k
		i++
	}

	sort.Strings(keys)
	return keys
}
