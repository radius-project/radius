// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

var _ renderers.Renderer = (*KubernetesRenderer)(nil)

// SupportedKubernetesPubSubKindValues is a map of supported resource kinds for k8s and the associated renderer
var SupportedKubernetesPubSubKindValues = map[string]PubSubFunc{
	resourcekinds.DaprPubSubTopicGeneric: GetDaprPubSubAzureGenericKubernetes,
}

type KubernetesRenderer struct {
	PubSubs map[string]PubSubFunc
}

func (r *KubernetesRenderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func GetDaprPubSubAzureGenericKubernetes(resource renderers.RendererResource) (renderers.RendererOutput, error) {
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

	pubsubResource, err := dapr.ConstructDaprGeneric(daprGeneric, resource.ApplicationName, resource.ResourceName)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	output := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDaprPubSubGeneric,
		ResourceKind: resourcekinds.Kubernetes,
		Managed:      false,
		Resource:     &pubsubResource,
	}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{output},
		ComputedValues: nil,
		SecretValues:   nil,
	}, nil
}

func (r *KubernetesRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
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
