// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"sync"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/health/handlers"
	"github.com/project-radius/radius/pkg/resourcekinds"

	"k8s.io/client-go/kubernetes"
)

func NewApplicationHealthModel(arm *armauth.ArmConfig, k8s kubernetes.Interface, wg *sync.WaitGroup) HealthModel {
	azureHandlers := map[string]handlers.HealthHandler{}
	// Add health check handlers for the resource types: https://github.com/project-radius/radius/issues/827
	// 	// TODO: Add health check handler for all resource kinds
	handlers := map[string]handlers.HealthHandler{
		resourcekinds.Deployment: handlers.NewKubernetesDeploymentHandler(k8s),
	}

	// Monitor health for Azure resources if Azure credentials have been provided
	if arm != nil {
		for k, v := range azureHandlers {
			handlers[k] = v
		}
	}
	return NewHealthModel(handlers, wg)
}
