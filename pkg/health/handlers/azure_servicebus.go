// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health/db"
	"github.com/Azure/radius/pkg/healthcontract"
)

func NewAzureServiceBusQueueHandler(arm armauth.ArmConfig) HealthHandler {
	handler := &azureServiceBusQueueHandler{
		azureServiceBusBaseHandler: azureServiceBusBaseHandler{arm: arm},
	}
	return handler
}

type azureServiceBusBaseHandler struct {
	arm armauth.ArmConfig
}

type azureServiceBusQueueHandler struct {
	azureServiceBusBaseHandler
}

func (handler *azureServiceBusBaseHandler) getQueueByID(ctx context.Context, id string) (*servicebus.SBQueue, error) {
	azureResource, err := azresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse servicebus resource id: %w", err)
	}
	qc := azclients.NewQueuesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	queue, err := qc.Get(ctx, azureResource.ResourceGroup, azureResource.Types[0].Name, azureResource.Types[1].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get servicebus queue: %w", err)
	}

	return &queue, nil
}

func (handler *azureServiceBusBaseHandler) GetHealthState(ctx context.Context, resourceInfo healthcontract.ResourceInfo, options healthcontract.HealthCheckOptions) healthcontract.ResourceHealthDataMessage {
	queue, err := handler.getQueueByID(ctx, resourceInfo.ResourceID)
	var healthState = db.Healthy
	var healthStateErrorDetails string
	if err != nil {
		healthState = db.Unhealthy
		healthStateErrorDetails = err.Error()
	} else if queue.Status != servicebus.Active {
		healthState = db.Unhealthy
		healthStateErrorDetails = fmt.Sprintf("Queue Status: %s", queue.Status)
	}

	healthData := healthcontract.ResourceHealthDataMessage{
		Resource:                resourceInfo,
		HealthState:             healthState,
		HealthStateErrorDetails: healthStateErrorDetails,
	}
	return healthData
}
