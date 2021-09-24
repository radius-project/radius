// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicebusqueuev1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
)

var _ renderers.AdaptableRenderer = (*Renderer)(nil)

// Renderer is the WorkloadRenderer implementation for the service bus workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for servicebus workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	if len(resources) != 1 || resources[0].Type != resourcekinds.AzureServiceBusQueue {
		return nil, fmt.Errorf("cannot fulfill binding - expected properties for %s", resourcekinds.AzureServiceBusQueue)
	}

	properties := resources[0].Properties
	namespaceName := properties[handlers.ServiceBusNamespaceNameKey]
	queueName := properties[handlers.ServiceBusQueueNameKey]
	namespaceConnectionString := properties[handlers.ServiceBusNamespaceConnectionStringKey]
	queueConnectionString := properties[handlers.ServiceBusQueueConnectionStringKey]

	bindings := map[string]components.BindingState{
		"default": {
			Component: workload.Name,
			Binding:   "default",
			Kind:      "azure.com/ServiceBusQueue",
			Properties: map[string]interface{}{
				"connectionString":          namespaceConnectionString,
				"namespaceConnectionString": namespaceConnectionString,
				"queueConnectionString":     queueConnectionString,
				"namespace":                 namespaceName,
				"queue":                     queueName,
			},
		},
	}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for servicebus workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := ServiceBusQueueComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return nil, err
	}

	if component.Config.Managed {
		if component.Config.Queue == "" {
			return nil, errors.New("the 'topic' field is required when 'managed=true'")
		}

		if component.Config.Resource != "" {
			return nil, renderers.ErrResourceSpecifiedForManagedResource
		}

		// generate data we can use to manage a servicebus queue

		resource := outputresource.OutputResource{
			LocalID: outputresource.LocalIDAzureServiceBusQueue,
			Kind:    resourcekinds.AzureServiceBusQueue,
			Type:    outputresource.TypeARM,
			Managed: true,
			Resource: map[string]string{
				handlers.ManagedKey:             "true",
				handlers.ServiceBusQueueNameKey: component.Config.Queue,
			},
		}

		// It's already in the correct format
		return []outputresource.OutputResource{resource}, nil
	} else {
		if component.Config.Resource == "" {
			return nil, renderers.ErrResourceMissingForUnmanagedResource
		}

		queueID, err := renderers.ValidateResourceID(component.Config.Resource, QueueResourceType, "ServiceBus Queue")
		if err != nil {
			return nil, err
		}

		// TODO : Need to create an output resource for service bus namespace

		resource := outputresource.OutputResource{
			LocalID: outputresource.LocalIDAzureServiceBusQueue,
			Kind:    resourcekinds.AzureServiceBusQueue,
			Type:    outputresource.TypeARM,
			Managed: false,
			Resource: map[string]string{
				handlers.ManagedKey: "false",

				// Truncate the queue part of the ID to make an ID for the namespace
				handlers.ServiceBusNamespaceIDKey:   azresources.MakeID(queueID.SubscriptionID, queueID.ResourceGroup, queueID.Types[0]),
				handlers.ServiceBusQueueIDKey:       queueID.ID,
				handlers.ServiceBusNamespaceNameKey: queueID.Types[0].Name,
				handlers.ServiceBusQueueNameKey:     queueID.Types[1].Name,
			},
		}

		// It's already in the correct format
		return []outputresource.OutputResource{resource}, nil
	}
}

func (r *Renderer) GetKind() string {
	return Kind
}
func (r *Renderer) GetComputedValues(ctx context.Context, workload workloads.InstantiatedWorkload) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference, error) {
	computedValues := map[string]renderers.ComputedValueReference{
		"namespace": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusNamespaceNameKey,
		},
		"queue": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusQueueNameKey,
		},
		"connectionString": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusNamespaceConnectionStringKey,
		},
		"namespaceConnectionString": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusNamespaceConnectionStringKey,
		},
		"queueConnectionString": {
			LocalID:           outputresource.LocalIDAzureServiceBusQueue,
			PropertyReference: handlers.ServiceBusQueueConnectionStringKey,
		},
	}
	secretValues := map[string]renderers.SecretValueReference{}

	return computedValues, secretValues, nil
}
