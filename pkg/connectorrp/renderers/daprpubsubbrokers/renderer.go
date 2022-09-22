// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
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
	// Check if Dapr object with the same componentName exist
	found, err := dapr.DoesComponentNameExist(ctx, resource.Properties.Application, resource.Name)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	if found {
		return renderers.RendererOutput{}, errors.New("There is already a Dapr component with the same name.")
	}

	if resource.Properties.Kind == "" {
		return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest("Resource kind not specified for Dapr Pub/Sub component")
	}

	if r.PubSubs == nil {
		return renderers.RendererOutput{}, errors.New("must support either kubernetes or ARM")
	}

	kind := string(resource.Properties.Kind)
	pubSubFunc, ok := r.PubSubs[kind]
	if !ok {
		return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("%s is not supported. Supported kind values: %s", kind, getAlphabeticallySortedKeys(r.PubSubs)))
	}

	var applicationName string
	if resource.Properties.Application != "" {
		applicationID, err := renderers.ValidateApplicationID(resource.Properties.Application)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		applicationName = applicationID.Name()
	}

	return pubSubFunc(*resource, applicationName, options.Namespace)
}

func getAlphabeticallySortedKeys(store map[string]PubSubFunc) []string {
	keys := make([]string, len(store))

	i := 0
	for k := range store {
		keys[i] = k
		i++
	}

	sort.Strings(keys)
	return keys
}
