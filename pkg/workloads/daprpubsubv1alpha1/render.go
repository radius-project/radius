// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/curp/handlers"
	"github.com/Azure/radius/pkg/curp/resources"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for the dapr pubsub workload.
type Renderer struct {
}

// Allocate is the WorkloadRenderer implementation for dapr pubsub workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if service.Kind != "dapr.io/PubSubTopic" {
		return nil, fmt.Errorf("cannot fulfill service kind: %v", service.Kind)
	}

	if len(wrp) != 1 || wrp[0].Type != workloads.ResourceKindDaprPubSubTopicAzureServiceBus {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for %s", workloads.ResourceKindDaprPubSubTopicAzureServiceBus)
	}

	properties := wrp[0].Properties
	namespaceName := properties[handlers.ServiceBusNamespaceNameKey]
	pubsubName := properties[handlers.KubernetesNameKey]
	topicName := properties[handlers.ServiceBusTopicNameKey]

	values := map[string]interface{}{
		"namespace":  namespaceName,
		"pubsubName": pubsubName,
		"topic":      topicName,
	}

	return values, nil
}

// Render is the WorkloadRenderer implementation for dapr pubsub workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := DaprPubSubComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	// The Dapr pubsub name can default to the component name.
	if component.Config.Name == "" {
		component.Config.Name = component.Name
	}

	if component.Config.Managed {
		if component.Config.Topic == "" {
			return []workloads.WorkloadResource{}, errors.New("the 'topic' field is required when 'managed=true'")
		}

		if component.Config.Resource != "" {
			return nil, errors.New("the 'resource' field cannot be specified when 'managed=true'")
		}

		// generate data we can use to manage a servicebus topic
		resource := workloads.WorkloadResource{
			Type: workloads.ResourceKindDaprPubSubTopicAzureServiceBus,
			Resource: map[string]string{
				handlers.ManagedKey:              "true",
				handlers.KubernetesNameKey:       component.Config.Name,
				handlers.KubernetesNamespaceKey:  w.Application,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ServiceBusTopicNameKey:  component.Config.Topic,
			},
		}

		return []workloads.WorkloadResource{resource}, nil
	} else {
		if component.Config.Topic != "" {
			return nil, errors.New("the 'topic' cannot be specified when 'managed' is not specified")
		}

		if component.Config.Resource == "" {
			return nil, errors.New("the 'resource' field is required when 'managed' is not specified")
		}

		topicID, err := radresources.Parse(component.Config.Resource)
		if err != nil {
			return nil, errors.New("the 'resource' field must be a valid resource id.")
		}

		err = topicID.ValidateResourceType(TopicResourceType)
		if err != nil {
			return nil, fmt.Errorf("the 'resource' field must refer to a ServiceBus Topic")
		}

		// generate data we can use to connect to a servicebus topic
		resource := workloads.WorkloadResource{
			Type: workloads.ResourceKindDaprPubSubTopicAzureServiceBus,
			Resource: map[string]string{
				handlers.ManagedKey:              "false",
				handlers.KubernetesNameKey:       component.Config.Name,
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
		return []workloads.WorkloadResource{resource}, nil
	}
}
