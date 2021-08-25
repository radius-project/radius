// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for the dapr pubsub workload.
type Renderer struct {
}

// AllocateBindings is the WorkloadRenderer implementation for dapr pubsub workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	if len(resources) != 1 || resources[0].Type != outputresource.KindDaprPubSubTopicAzureServiceBus {
		return nil, fmt.Errorf("cannot fulfill binding - expected properties for %s", outputresource.KindDaprPubSubTopicAzureServiceBus)
	}

	properties := resources[0].Properties
	namespaceName := properties[handlers.ServiceBusNamespaceNameKey]
	pubsubName := properties[handlers.ComponentNameKey]
	topicName := properties[handlers.ServiceBusTopicNameKey]

	bindings := map[string]components.BindingState{
		"default": {
			Component: workload.Name,
			Binding:   "default",
			Kind:      "dapr.io/PubSubTopic",
			Properties: map[string]interface{}{
				"namespace":  namespaceName,
				"pubSubName": pubsubName,
				"topic":      topicName,
			},
		},
	}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for dapr pubsub workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := DaprPubSubComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	// The Dapr pubsub name can default to the component name.
	if component.Config.Name == "" {
		component.Config.Name = component.Name
	}

	if component.Config.Managed {
		if component.Config.Topic == "" {
			return []outputresource.OutputResource{}, errors.New("the 'topic' field is required when 'managed=true'")
		}

		if component.Config.Resource != "" {
			return nil, workloads.ErrResourceSpecifiedForManagedResource
		}

		// generate data we can use to manage a servicebus topic
		resource := outputresource.OutputResource{
			LocalID: outputresource.LocalIDAzureServiceBusTopic,
			Kind:    outputresource.KindDaprPubSubTopicAzureServiceBus,
			Type:    outputresource.TypeARM,
			Managed: true,
			Resource: map[string]string{
				handlers.ManagedKey:              "true",
				handlers.ComponentNameKey:        component.Config.Name,
				handlers.KubernetesNamespaceKey:  w.Application,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ServiceBusTopicNameKey:  component.Config.Topic,
			},
		}

		return []outputresource.OutputResource{resource}, nil
	} else {
		if component.Config.Topic != "" {
			return nil, errors.New("the 'topic' cannot be specified when 'managed' is not specified")
		}

		if component.Config.Resource == "" {
			return nil, workloads.ErrResourceMissingForUnmanagedResource
		}

		topicID, err := workloads.ValidateResourceID(component.Config.Resource, TopicResourceType, "ServiceBus Topic")
		if err != nil {
			return nil, err
		}

		resource := outputresource.OutputResource{
			LocalID: outputresource.LocalIDAzureServiceBusTopic,
			Kind:    outputresource.KindDaprPubSubTopicAzureServiceBus,
			Type:    outputresource.TypeARM,
			Managed: false,
			Resource: map[string]string{
				handlers.ManagedKey:              "false",
				handlers.ComponentNameKey:        component.Config.Name,
				handlers.KubernetesNamespaceKey:  w.Application,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",

				// Truncate the topic part of the ID to make an ID for the namespace
				handlers.ServiceBusNamespaceIDKey:   azresources.MakeID(topicID.SubscriptionID, topicID.ResourceGroup, topicID.Types[0]),
				handlers.ServiceBusTopicIDKey:       topicID.ID,
				handlers.ServiceBusNamespaceNameKey: topicID.Types[0].Name,
				handlers.ServiceBusTopicNameKey:     topicID.Types[1].Name,
			},
		}
		return []outputresource.OutputResource{resource}, nil
	}
}
