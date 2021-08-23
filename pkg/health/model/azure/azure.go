// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/model"
)

func NewAzureHealthModel(arm armauth.ArmConfig) model.HealthModel {
	// Add health check handlers for the resource types
	handlers := map[string]handlers.HealthHandler{
		// TODO: Add health check handler for all resource kinds
		ResourceKindAzureServiceBusQueue: handlers.NewAzureServiceBusQueueHandler(arm),
	}
	return model.NewHealthModel(handlers)
}
