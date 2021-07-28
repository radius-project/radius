// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha1

import (
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

func GetAzureRedis(w workloads.InstantiatedWorkload, component RedisComponent) ([]workloads.OutputResource, error) {

	if component.Config.Managed {
		resource := workloads.OutputResource{
			LocalID:            workloads.LocalIDAzureRedis,
<<<<<<< HEAD
			ResourceKind:       workloads.KindAzureRedis,
			OutputResourceType: workloads.OutputResourceTypeArm,
			Managed:            true,
			Resource: map[string]string{
				handlers.ManagedKey:    "true",
				handlers.RedisBaseName: w.Workload.Name,
=======
			ResourceKind:       workloads.ResourceKindAzureRedis,
			OutputResourceType: workloads.OutputResourceTypeArm,
			Managed:            true,
			Resource: map[string]string{
				handlers.ManagedKey:        "true",
				handlers.AzureRedisNameKey: component.Config.Name,
>>>>>>> ae02a78 (Refactor for k8s and azure)
			},
		}
		return []workloads.OutputResource{resource}, nil
	} else {
		// TODO
	}

	return []workloads.OutputResource{}, nil
}
