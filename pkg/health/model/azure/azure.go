// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/model"
	"github.com/Azure/radius/pkg/healthcontract"
	k8s "k8s.io/client-go/kubernetes"
)

func NewAzureHealthModel(arm armauth.ArmConfig, k8s *k8s.Clientset) model.HealthModel {
	// Add health check handlers for the resource types
	handlers := map[string]handlers.HealthHandler{
		// TODO: Add health check handler for all resource kinds
		ResourceKindAzureServiceBusQueue:        handlers.NewAzureServiceBusQueueHandler(arm),
		healthcontract.KubernetesKindDeployment: handlers.NewKubernetesDeploymentHandler(*k8s),
		healthcontract.KubernetesKindService:    handlers.NewKubernetesServiceHandler(*k8s),
	}
	return model.NewHealthModel(handlers)
}
