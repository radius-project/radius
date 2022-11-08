// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func GetDaprPubSubAzureServiceBus(resource datamodel.DaprPubSubBroker, applicationName string, namespace string) (renderers.RendererOutput, error) {
	properties := resource.Properties
	var output outputresource.OutputResource

	if properties.Resource == "" {
		return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest(renderers.ErrResourceMissingForResource.Error())
	}

	//Validate fully qualified resource identifier of the source resource is supplied for this link
	azureServiceBusNamespaceID, err := resources.ParseResource(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
	}

	err = azureServiceBusNamespaceID.ValidateResourceType(NamespaceResourceType)
	if err != nil {
		return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest("the 'resource' field must refer to a ServiceBus Namespace")
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
			Provider: resourcemodel.ProviderAzure,
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
		renderers.ComponentNameKey: {
			Value: kubernetes.MakeResourceName(applicationName, resource.Name),
		},
	}

	secrets := map[string]rp.SecretValueReference{}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{output},
		ComputedValues: values,
		SecretValues:   secrets,
	}, nil
}
