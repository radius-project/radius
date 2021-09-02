// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/model"
	"github.com/Azure/radius/pkg/health/resourcekinds"
	"k8s.io/client-go/kubernetes"
)

func NewAzureHealthModel(arm armauth.ArmConfig, k8s kubernetes.Interface) model.HealthModel {
	// Add health check handlers for the resource types: https://github.com/Azure/radius/issues/827
	handlers := map[string]handlers.HealthHandler{
		// TODO: Add health check handler for all resource kinds
		resourcekinds.ResourceKindAzureServiceBusQueue: handlers.NewAzureServiceBusQueueHandler(arm),
		resourcekinds.KubernetesKindDeployment:         handlers.NewKubernetesDeploymentHandler(k8s),
		resourcekinds.KubernetesKindService:            handlers.NewKubernetesServiceHandler(k8s),
	}
	return model.NewHealthModel(handlers)
}
