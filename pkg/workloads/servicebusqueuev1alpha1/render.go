// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicebusqueuev1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for the service bus workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for servicebus workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if service.Kind != "azure.com/ServiceBusQueue" {
		return nil, fmt.Errorf("cannot fulfill service kind: %v", service.Kind)
	}

	if len(wrp) != 1 || wrp[0].Type != workloads.ResourceKindAzureServiceBusQueue {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for %s", workloads.ResourceKindAzureServiceBusQueue)
	}

	properties := wrp[0].Properties
	namespaceName := properties["servicebusnamespace"]
	queueName := properties["servicebusqueue"]

	sbClient := servicebus.NewNamespacesClient(r.Arm.SubscriptionID)
	sbClient.Authorizer = r.Arm.Auth
	accessKeys, err := sbClient.ListKeys(ctx, r.Arm.ResourceGroup, namespaceName, "RootManageSharedAccessKey")

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if accessKeys.PrimaryConnectionString == nil && accessKeys.SecondaryConnectionString == nil {
		return nil, fmt.Errorf("failed to retrieve connection strings")
	}

	cs := accessKeys.PrimaryConnectionString

	values := map[string]interface{}{
		"connectionString": *cs,
		"namespace":        namespaceName,
		"queue":            queueName,
	}

	return values, nil
}

// Render is the WorkloadRenderer implementation for servicebus workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := ServiceBusQueueComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if !component.Config.Managed {
		return []workloads.WorkloadResource{}, errors.New("only 'managed=true' is supported right now")
	}

	if component.Config.Managed && component.Config.Queue == "" {
		return []workloads.WorkloadResource{}, errors.New("the 'topic' field is require when 'managed=true'")
	}

	// generate data we can use to manage a servicebus instance

	resource := workloads.WorkloadResource{
		Type: workloads.ResourceKindAzureServiceBusQueue,
		Resource: map[string]string{
			"servicebusname":  w.Workload.Name,
			"servicebusqueue": component.Config.Queue,
		},
	}

	// It's already in the correct format
	return []workloads.WorkloadResource{resource}, nil
}
