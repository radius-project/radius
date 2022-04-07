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
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

var _ renderers.Renderer = (*Renderer)(nil)

type PubSubFunc = func(renderers.RendererResource) (renderers.RendererOutput, error)

const (
	appName           = "test-app"
	resourceName      = "test-resource"
	pubsubType        = "pubsub.kafka"
	daprPubSubVersion = "v1"
	daprVersion       = "dapr.io/v1alpha1"
	k8sKind           = "Component"
)

// SupportedAzurePubSubKindValues is a map of supported resource kinds for Azure and the associated renderer
var SupportedPubSubKindValues = map[string]PubSubFunc{
	resourcekinds.DaprPubSubTopicAzureServiceBus: GetDaprPubSubAzureServiceBus,
	resourcekinds.DaprGeneric:                    GetDaprPubSubGeneric,
}

type Renderer struct {
	PubSubs map[string]PubSubFunc
}

type Properties struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
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
