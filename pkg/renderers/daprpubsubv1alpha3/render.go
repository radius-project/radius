// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"context"
	"errors"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource := options.Resource

	properties := Properties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resources := []outputresource.OutputResource{}
	if properties.Managed {
		if properties.Topic == "" {
			return renderers.RendererOutput{}, errors.New("the 'topic' field is required when 'managed=true'")
		}

		if properties.Resource != "" {
			return renderers.RendererOutput{}, renderers.ErrResourceSpecifiedForManagedResource
		}

		// generate data we can use to manage a servicebus topic
		output := outputresource.OutputResource{
			LocalID:      outputresource.LocalIDAzureServiceBusTopic,
			ResourceKind: resourcekinds.DaprPubSubTopicAzureServiceBus,
			Managed:      true,
			Resource: map[string]string{
				handlers.ManagedKey:              "true",
				handlers.ResourceName:            resource.ResourceName,
				handlers.KubernetesNamespaceKey:  resource.ApplicationName,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ServiceBusTopicNameKey:  properties.Topic,
			},
		}

		resources = append(resources, output)
	} else {
		if properties.Topic != "" {
			return renderers.RendererOutput{}, errors.New("the 'topic' cannot be specified when 'managed' is not specified")
		}

		if properties.Resource == "" {
			return renderers.RendererOutput{}, renderers.ErrResourceMissingForUnmanagedResource
		}

		topicID, err := renderers.ValidateResourceID(properties.Resource, TopicResourceType, "ServiceBus Topic")
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		output := outputresource.OutputResource{
			LocalID:      outputresource.LocalIDAzureServiceBusTopic,
			ResourceKind: resourcekinds.DaprPubSubTopicAzureServiceBus,
			Managed:      false,
			Resource: map[string]string{
				handlers.ManagedKey:              "false",
				handlers.ResourceName:            resource.ResourceName,
				handlers.KubernetesNamespaceKey:  resource.ApplicationName,
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",

				// Truncate the topic part of the ID to make an ID for the namespace
				handlers.ServiceBusNamespaceIDKey:   topicID.Truncate().ID,
				handlers.ServiceBusTopicIDKey:       topicID.ID,
				handlers.ServiceBusNamespaceNameKey: topicID.Types[0].Name,
				handlers.ServiceBusTopicNameKey:     topicID.Types[1].Name,
			},
		}

		resources = append(resources, output)
	}

	values := map[string]renderers.ComputedValueReference{
		"namespace": {
			LocalID:           outputresource.LocalIDAzureServiceBusTopic,
			PropertyReference: handlers.ServiceBusNamespaceNameKey,
		},
		"pubSubName": {
			LocalID:           outputresource.LocalIDAzureServiceBusTopic,
			PropertyReference: handlers.ResourceName,
		},
		"topic": {
			LocalID:           outputresource.LocalIDAzureServiceBusTopic,
			PropertyReference: handlers.ServiceBusTopicNameKey,
		},
	}
	secrets := map[string]renderers.SecretValueReference{}

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: values,
		SecretValues:   secrets,
	}, nil
}
