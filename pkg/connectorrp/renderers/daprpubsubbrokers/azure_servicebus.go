// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func GetDaprPubSubAzureServiceBus(dm conv.DataModelInterface) (renderers.RendererOutput, error) {
	resource, ok := dm.(datamodel.DaprPubSubBroker)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}
	properties := resource.Properties.DaprPubSubAzureServiceBus

	var output outputresource.OutputResource

	if properties.Resource == "" {
		return renderers.RendererOutput{}, renderers.ErrResourceMissingForResource
	}
	//Validate fully qualified resource identifier of the source resource is supplied for this connector
	azureServiceBusTopicID, err := resources.Parse(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, errors.New("the 'resource' field must be a valid resource id")
	}
	err = azureServiceBusTopicID.ValidateResourceType(TopicResourceType)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("the 'resource' field must refer to a ServiceBus Topic")
	}
	output = outputresource.OutputResource{
		LocalID: outputresource.LocalIDAzureServiceBusTopic,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
			Provider: providers.ProviderAzure,
		},
		Resource: map[string]string{
			handlers.ResourceName:            resource.Name,
			handlers.KubernetesNamespaceKey:  resource.Properties.Application,
			handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
			handlers.KubernetesKindKey:       "Component",

			// Truncate the topic part of the ID to make an ID for the namespace
			handlers.ServiceBusNamespaceIDKey:   azureServiceBusTopicID.Truncate().String(),
			handlers.ServiceBusTopicIDKey:       azureServiceBusTopicID.String(),
			handlers.ServiceBusNamespaceNameKey: azureServiceBusTopicID.TypeSegments()[0].Name,
			handlers.ServiceBusTopicNameKey:     azureServiceBusTopicID.TypeSegments()[1].Name,
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
	secrets := map[string]rp.SecretValueReference{}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{output},
		ComputedValues: values,
		SecretValues:   secrets,
	}, nil
}
