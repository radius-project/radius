// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

func GetDaprPubSubAzureServiceBus(resource renderers.RendererResource) (renderers.RendererOutput, error) {
	properties := radclient.DaprPubSubTopicAzureServiceBusResourceProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	var output outputresource.OutputResource

	if to.String(properties.Resource) == "" {
		return renderers.RendererOutput{}, renderers.ErrResourceMissingForResource
	}

	topicID, err := renderers.ValidateResourceID(to.String(properties.Resource), TopicResourceType, "ServiceBus Topic")
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	output = outputresource.OutputResource{
		LocalID: outputresource.LocalIDAzureServiceBusTopic,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
			Provider: providers.ProviderAzure,
		},
		Resource: map[string]string{
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
		Resources:      []outputresource.OutputResource{output},
		ComputedValues: values,
		SecretValues:   secrets,
	}, nil
}
