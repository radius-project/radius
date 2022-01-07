// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"sync"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/model"
	"github.com/Azure/radius/pkg/resourcekinds"
	"k8s.io/client-go/kubernetes"
)

func NewAzureHealthModel(arm armauth.ArmConfig, k8s kubernetes.Interface, wg *sync.WaitGroup) model.HealthModel {
	// Add health check handlers for the resource types: https://github.com/project-radius/radius/issues/827
	handlers := map[string]handlers.HealthHandler{
		// TODO: Add health check handler for all resource kinds
		resourcekinds.AzureServiceBusQueue: handlers.NewAzureServiceBusQueueHandler(arm),
		resourcekinds.Deployment:           handlers.NewKubernetesDeploymentHandler(k8s),
	}
	return model.NewHealthModel(handlers, wg)
}
