/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package daprpubsubbrokers

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func GetDaprPubSubAzureServiceBus(resource datamodel.DaprPubSubBroker, applicationName string, namespace string) (renderers.RendererOutput, error) {
	properties := resource.Properties
	var output rpv1.OutputResource

	if properties.Resource == "" {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(renderers.ErrResourceMissingForResource.Error())
	}

	//Validate fully qualified resource identifier of the source resource is supplied for this link
	azureServiceBusNamespaceID, err := resources.ParseResource(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
	}

	err = azureServiceBusNamespaceID.ValidateResourceType(NamespaceResourceType)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("the 'resource' field must refer to a ServiceBus Namespace")
	}

	serviceBusNamespaceName := azureServiceBusNamespaceID.TypeSegments()[0].Name

	topicName := resource.Properties.Topic
	if topicName == "" {
		topicName = resource.Name
	}

	output = rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureServiceBusNamespace,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
			Provider: resourcemodel.ProviderAzure,
		},
		Resource: map[string]string{
			handlers.ResourceName:            kubernetes.NormalizeDaprResourceName(resource.Name),
			handlers.KubernetesNamespaceKey:  namespace,
			handlers.ApplicationName:         kubernetes.NormalizeResourceName(applicationName),
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
			Value:             kubernetes.NormalizeDaprResourceName(resource.Name),
			LocalID:           rpv1.LocalIDAzureServiceBusNamespace,
			PropertyReference: handlers.ResourceName,
		},
		TopicNameKey: {
			Value: topicName,
		},
		renderers.ComponentNameKey: {
			Value: kubernetes.NormalizeDaprResourceName(resource.Name),
		},
	}

	secrets := map[string]rpv1.SecretValueReference{}

	return renderers.RendererOutput{
		Resources:      []rpv1.OutputResource{output},
		ComputedValues: values,
		SecretValues:   secrets,
	}, nil
}
