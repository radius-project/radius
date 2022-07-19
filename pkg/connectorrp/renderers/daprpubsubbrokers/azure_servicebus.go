// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/handlers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func GetDaprPubSubAzureServiceBus(resource datamodel.DaprPubSubBroker, applicationName string, namespace string) (renderers.RendererOutput, error) {
	properties := resource.Properties.DaprPubSubAzureServiceBus

	var output outputresource.OutputResource

	if properties.Resource == "" {
		return renderers.RendererOutput{}, renderers.ErrResourceMissingForResource
	}
	//Validate fully qualified resource identifier of the source resource is supplied for this connector
	azureServiceBusNamespaceID, err := resources.Parse(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, errors.New("the 'resource' field must be a valid resource id")
	}
	err = azureServiceBusNamespaceID.ValidateResourceType(NamespaceResourceType)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("the 'resource' field must refer to a ServiceBus Namespace")
	}

	serviceBusNamespaceName := azureServiceBusNamespaceID.TypeSegments()[0].Name
	topicName := resource.Properties.Topic
	if topicName == "" {
		topicName = resource.Name
	}
	output = outputresource.OutputResource{
		LocalID: outputresource.LocalIDAzureServiceBusNamespace,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
			Provider: providers.ProviderAzure,
		},
		Resource: map[string]string{
			handlers.ResourceName:            resource.Name,
			handlers.KubernetesNamespaceKey:  namespace,
			handlers.ApplicationName:         applicationName,
			handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
			handlers.KubernetesKindKey:       "Component",

			handlers.ServiceBusNamespaceIDKey:   azureServiceBusNamespaceID.String(),
			handlers.ServiceBusNamespaceNameKey: serviceBusNamespaceName,
			handlers.ServiceBusTopicNameKey:     topicName,
		},
	}

	values := map[string]renderers.ComputedValueReference{
		NamespaceNameKey: {
			Value: serviceBusNamespaceName,
		},
		PubSubNameKey: {
			Value:             kubernetes.MakeResourceName(applicationName, resource.Name),
			LocalID:           outputresource.LocalIDAzureServiceBusNamespace,
			PropertyReference: handlers.ResourceName,
		},
		TopicNameKey: {
			Value: topicName,
		},
	}
	secrets := map[string]rp.SecretValueReference{}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{output},
		ComputedValues: values,
		SecretValues:   secrets,
	}, nil
}
