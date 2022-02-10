// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

var _ renderers.Renderer = (*Renderer)(nil)

type PubSubFunc = func(renderers.RendererResource) (renderers.RendererOutput, error)

// SupportedAzurePubSubKindValues is a map of supported resource kinds for Azure and the associated renderer
var SupportedAzurePubSubKindValues = map[string]PubSubFunc{
	resourcekinds.DaprPubSubTopicAzureServiceBus: GetDaprPubSubAzureServiceBus,
	resourcekinds.DaprPubSubTopicGeneric:         GetDaprPubSubAzureGeneric,
}

type Renderer struct {
	PubSubs map[string]PubSubFunc
}

type Properties struct {
	Kind     string `json:"kind"`
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func GetDaprPubSubAzureServiceBus(resource renderers.RendererResource) (renderers.RendererOutput, error) {
	properties := radclient.DaprPubSubTopicAzureServiceBusResourceProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	var output outputresource.OutputResource
	if to.String(properties.Topic) != "" {
		return renderers.RendererOutput{}, errors.New("the 'topic' cannot be specified when 'managed' is not specified")
	}

	if to.String(properties.Resource) == "" {
		return renderers.RendererOutput{}, renderers.ErrResourceMissingForUnmanagedResource
	}

	topicID, err := renderers.ValidateResourceID(to.String(properties.Resource), TopicResourceType, "ServiceBus Topic")
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	output = outputresource.OutputResource{
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

func GetDaprPubSubAzureGeneric(resource renderers.RendererResource) (renderers.RendererOutput, error) {
	properties := radclient.DaprPubSubTopicGenericResourceProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	daprGeneric := dapr.DaprGeneric{
		Type:     properties.Type,
		Version:  properties.Version,
		Metadata: properties.Metadata,
	}

	err = dapr.ValidateDaprGenericObject(daprGeneric)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Convert metadata to string
	metadataSerialized, err := json.Marshal(properties.Metadata)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	output := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDaprPubSubGeneric,
		ResourceKind: resourcekinds.DaprPubSubTopicGeneric,
		Managed:      false,
		Resource: map[string]string{
			handlers.ManagedKey:              "false",
			handlers.ResourceName:            resource.ResourceName,
			handlers.KubernetesNamespaceKey:  resource.ApplicationName,
			handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
			handlers.KubernetesKindKey:       "Component",

			handlers.GenericDaprTypeKey:     *properties.Type,
			handlers.GenericDaprVersionKey:  *properties.Version,
			handlers.GenericDaprMetadataKey: string(metadataSerialized),
		},
	}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{output},
		ComputedValues: nil,
		SecretValues:   nil,
	}, nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource := options.Resource

	if _, ok := resource.Definition["kind"]; !ok {
		return renderers.RendererOutput{}, errors.New("Resource kind not specified for Dapr Pub/Sub component")
	}

	kind := resource.Definition["kind"].(string)

	if r.PubSubs == nil {
		return renderers.RendererOutput{}, errors.New("must support either kubernetes or ARM")
	}

	pubSubFunc, ok := r.PubSubs[kind]
	if !ok {
		return renderers.RendererOutput{}, fmt.Errorf("Renderer not found for kind: %s", kind)
	}

	return pubSubFunc(resource)
}
