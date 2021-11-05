// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprhttproutev1alpha3

import (
	"context"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/renderers"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.DaprHTTPRouteProperties{}
	err := options.Resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	appId := ""
	if properties.AppID != nil {
		appId = *properties.AppID
	}
	return renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"appId": {
				Value: appId,
			},
		},
	}, nil
}
