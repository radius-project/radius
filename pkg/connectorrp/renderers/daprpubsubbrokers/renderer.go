// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ renderers.Renderer = (*Renderer)(nil)

type PubSubFunc = func(resource datamodel.DaprPubSubBroker, applicationName string, namespace string) (renderers.RendererOutput, error)

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

func (r *Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.DaprPubSubBroker)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	if resource.Properties.Kind == "" {
		return renderers.RendererOutput{}, errors.New("Resource kind not specified for Dapr Pub/Sub component")
	}

	if r.PubSubs == nil {
		return renderers.RendererOutput{}, errors.New("must support either kubernetes or ARM")
	}

	kind := string(resource.Properties.Kind)
	pubSubFunc, ok := r.PubSubs[kind]
	if !ok {
		return renderers.RendererOutput{}, fmt.Errorf("Renderer not found for kind: %s", kind)
	}

	var applicationName string
	if resource.Properties.Application != "" {
		applicationID, err := resources.Parse(resource.Properties.Application)
		if err != nil {
			return renderers.RendererOutput{}, errors.New("the 'application' field must be a valid resource id")
		}
		applicationName = applicationID.Name()
	}

	return pubSubFunc(*resource, applicationName, options.Namespace)
}
