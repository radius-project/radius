// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha1

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
)

func GetAzureRedis(w workloads.InstantiatedWorkload, component RedisComponent) ([]outputresource.OutputResource, error) {
	if component.Config.Managed {
		resource := outputresource.OutputResource{
			LocalID: outputresource.LocalIDAzureRedis,
			Kind:    outputresource.KindAzureRedis,
			Type:    outputresource.TypeARM,
			Managed: true,
			Resource: map[string]string{
				handlers.ManagedKey:    "true",
				handlers.RedisBaseName: w.Workload.Name,
			},
		}
		return []outputresource.OutputResource{resource}, nil
	} else {
		// TODO support managed redis workload
		return nil, fmt.Errorf("only managed = true is support for azure redis workload")
	}
}

func AllocateAzureBindings(arm armauth.ArmConfig, ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	properties := resources[0].Properties
	redisName := properties[handlers.RedisNameKey]

	rc := azclients.NewRedisClient(arm.SubscriptionID, arm.Auth)

	resource, err := rc.Get(ctx, arm.ResourceGroup, redisName)
	if err != nil {
		return nil, fmt.Errorf("failed to ge resource: %w", err)
	}

	accessKeys, err := rc.ListKeys(ctx, arm.ResourceGroup, redisName)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve keys: %w", err)
	}

	port := fmt.Sprint(*resource.Port)
	bindings := map[string]components.BindingState{
		"redis": {
			Component: workload.Name,
			Binding:   "redis",
			Kind:      BindingKind,
			Properties: map[string]interface{}{
				"connectionString": *resource.HostName + ":" + port,
				"host":             *resource.HostName,
				"port":             port,
				"primaryKey":       *accessKeys.PrimaryKey,
				"secondarykey":     *accessKeys.SecondaryKey,
			},
		},
	}
	return bindings, nil
}
