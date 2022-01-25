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
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	if properties.Type == nil || *properties.Type == "" {
		return renderers.RendererOutput{}, errors.New("No type specified for generic Dapr Pub/Sub component")
	}

	if properties.Version == nil || *properties.Version == "" {
		return renderers.RendererOutput{}, errors.New("No Dapr component version specified for generic Pub/Sub component")
	}

	if properties.Metadata == nil || len(properties.Metadata) == 0 {
		return renderers.RendererOutput{}, fmt.Errorf("No metadata specified for Dapr Pub/Sub component of type %s", *properties.Type)
	}

	pubsubResource, err := constructPubSubResource(properties, resource.ApplicationName, resource.ResourceName)
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

func constructPubSubResource(properties radclient.DaprPubSubTopicGenericResourceProperties, appName string, resourceName string) (unstructured.Unstructured, error) {
	// Convert the metadata map to a yaml list with keys name and value as per
	// Dapr specs: https://docs.dapr.io/reference/components-reference/supported-pubsub/
	yamlListItems := []map[string]interface{}{}
	for k, v := range properties.Metadata {
		yamlItem := map[string]interface{}{
			"name":  k,
			"value": v,
		}
		yamlListItems = append(yamlListItems, yamlItem)
	}

	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "dapr.io/v1alpha1",
			"kind":       "Component",
			"metadata": map[string]interface{}{
				"namespace": appName,
				"name":      resourceName,
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName),
			},
			"spec": map[string]interface{}{
				"type":     *properties.Type,
				"version":  *properties.Version,
				"metadata": yamlListItems,
			},
		},
	}

	return item, nil
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
