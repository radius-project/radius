package daprpubsubv1alpha3

import (
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/renderers/dapr"
)

const (
	appName           = "test-app"
	resourceName      = "test-resource"
	pubsubType        = "pubsub.kafka"
	daprPubSubVersion = "v1"
	daprVersion       = "dapr.io/v1alpha1"
	k8sKind           = "Component"
)

func GetDaprPubSubGeneric(resource renderers.RendererResource) (renderers.RendererOutput, error) {
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

	outputResources, err := dapr.GetDaprGeneric(daprGeneric, resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: nil,
		SecretValues:   nil,
	}, nil

}
