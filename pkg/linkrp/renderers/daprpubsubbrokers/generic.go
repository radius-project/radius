// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"errors"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

func GetDaprPubSubGeneric(resource datamodel.DaprPubSubBroker, applicationName string, namespace string) (renderers.RendererOutput, error) {
	properties := resource.Properties

	daprGeneric := dapr.DaprGeneric{
		Type:     &properties.Type,
		Version:  &properties.Version,
		Metadata: properties.Metadata,
	}

	outputResources, err := getDaprGeneric(daprGeneric, resource, applicationName, namespace)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	topicName := resource.Properties.Topic
	if topicName == "" {
		topicName = resource.Name
	}

	values := map[string]renderers.ComputedValueReference{
		NamespaceNameKey: {
			Value: namespace,
		},
		PubSubNameKey: {
			Value:             kubernetes.NormalizeResourceName(resource.Name),
			LocalID:           outputresource.LocalIDDaprComponent,
			PropertyReference: handlers.ResourceName,
		},
		TopicNameKey: {
			Value: topicName,
			Transformer: func(r conv.DataModelInterface, cv map[string]any) error {
				topicName, ok := cv[TopicNameKey].(string)
				if !ok {
					return errors.New("topic name must be set on computed values for DaprPubSubBroker")
				}
				res, ok := r.(*datamodel.DaprPubSubBroker)
				if !ok {
					return errors.New("resource must be DaprPubSubBroker")
				}

				res.Properties.Topic = topicName
				return nil
			},
		},
		renderers.ComponentNameKey: {
			Value: kubernetes.NormalizeResourceName(resource.Name),
			Transformer: func(r conv.DataModelInterface, cv map[string]any) error {
				componentName, ok := cv[renderers.ComponentNameKey].(string)
				if !ok {
					return errors.New("component name must be set on computed values for DaprPubSubBroker")
				}
				res, ok := r.(*datamodel.DaprPubSubBroker)
				if !ok {
					return errors.New("resource must be DaprPubSubBroker")
				}

				res.Properties.ComponentName = componentName
				return nil
			},
		},
	}

	secrets := map[string]rp.SecretValueReference{}

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: values,
		SecretValues:   secrets,
	}, nil

}

func getDaprGeneric(daprGeneric dapr.DaprGeneric, resource datamodel.DaprPubSubBroker, applicationName string, namespace string) ([]outputresource.OutputResource, error) {
	err := daprGeneric.Validate()
	if err != nil {
		return nil, err
	}

	daprGenericResource, err := dapr.ConstructDaprGeneric(daprGeneric, applicationName, resource.Name, namespace, resource.ResourceTypeName())
	if err != nil {
		return nil, err
	}

	output := outputresource.OutputResource{
		LocalID: outputresource.LocalIDDaprComponent,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprComponent,
			Provider: resourcemodel.ProviderKubernetes,
		},
		Resource: &daprGenericResource,
	}

	return []outputresource.OutputResource{output}, nil
}
