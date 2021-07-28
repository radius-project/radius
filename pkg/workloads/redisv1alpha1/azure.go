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
			ResourceKind:       workloads.KindAzureRedis,
			OutputResourceType: workloads.OutputResourceTypeArm,
			Managed:            true,
			Resource: map[string]string{
				handlers.ManagedKey:    "true",
				handlers.RedisBaseName: w.Workload.Name,
			},
		}
		return []workloads.OutputResource{resource}, nil
	} else {
		// TODO
	}

	return []workloads.OutputResource{}, nil
}
