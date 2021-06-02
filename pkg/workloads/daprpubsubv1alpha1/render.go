// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for the dapr pubsub workload.
type Renderer struct {
}

// Allocate is the WorkloadRenderer implementation for dapr pubsub workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	if len(resources) != 1 || resources[0].Type != workloads.ResourceKindDaprPubSubTopicAzureServiceBus {
		return nil, fmt.Errorf("cannot fulfill binding - expected properties for %s", workloads.ResourceKindDaprPubSubTopicAzureServiceBus)
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
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.OutputResource, error) {
	component := DaprPubSubComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.OutputResource{}, err
	}

	// The Dapr pubsub name can default to the component name.
	if component.Config.Name == "" {
		component.Config.Name = component.Name
	}

	if component.Config.Managed {
		if component.Config.Topic == "" {
			return []workloads.OutputResource{}, errors.New("the 'topic' field is required when 'managed=true'")
		}

		if component.Config.Resource != "" {
			return nil, workloads.ErrResourceSpecifiedForManagedResource
		}

		// generate data we can use to manage a servicebus topic
		resource := workloads.OutputResource{
			ResourceKind: workloads.ResourceKindDaprPubSubTopicAzureServiceBus,
			Resource: map[string]string{
				handlers.ManagedKey:              "true",
				handlers.ComponentNameKey:        component.Config.Name,
				handlers.KubernetesNamespaceKey:  w.Application,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ServiceBusTopicNameKey:  component.Config.Topic,
			},
		}

		return []workloads.OutputResource{resource}, nil
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

		// generate data we can use to connect to a servicebus topic
		resource := workloads.WorkloadResource{
			Type: workloads.ResourceKindDaprPubSubTopicAzureServiceBus,
			Resource: map[string]string{
				handlers.ManagedKey:              "false",
				handlers.ComponentNameKey:        component.Config.Name,
				handlers.KubernetesNamespaceKey:  w.Application,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",

				// Truncate the topic part of the ID to make an ID for the namespace
				handlers.ServiceBusNamespaceIDKey:   resources.MakeID(topicID.SubscriptionID, topicID.ResourceGroup, topicID.Types[0]),
				handlers.ServiceBusTopicIDKey:       topicID.ID,
				handlers.ServiceBusNamespaceNameKey: topicID.Types[0].Name,
				handlers.ServiceBusTopicNameKey:     topicID.Types[1].Name,
			},
		}
		return []workloads.WorkloadResource{resource}, []rest.RadResource{}, nil
	}
}
