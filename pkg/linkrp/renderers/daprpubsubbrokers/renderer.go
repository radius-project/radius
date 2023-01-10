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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

var _ renderers.Renderer = (*Renderer)(nil)

type PubSubFunc = func(resource datamodel.DaprPubSubBroker, applicationName string, namespace string) (renderers.RendererOutput, error)

var SupportedPubSubModes = map[string]PubSubFunc{
	string(datamodel.LinkModeResource): GetDaprPubSubAzureServiceBus,
	string(datamodel.LinkModeValues):   GetDaprPubSubGeneric,
}

type Renderer struct {
	PubSubs map[string]PubSubFunc
}

type Properties struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
}

func (r *Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.DaprPubSubBroker)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}
	if resource.Properties.Mode == "" {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("Mode not specified for Dapr Pub/Sub component")
	}
	if r.PubSubs == nil {
		return renderers.RendererOutput{}, errors.New("must support either kubernetes or ARM")
	}
	mode := resource.Properties.Mode
	pubSubFunc, ok := r.PubSubs[string(mode)]
	if !ok {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid pub sub broker mode, Supported mode values: %s", getAlphabeticallySortedKeys(r.PubSubs)))
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
