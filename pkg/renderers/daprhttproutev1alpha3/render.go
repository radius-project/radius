// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprhttproutev1alpha3

import (
	"context"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/renderers"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := DaprHttpRouteProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"appId": {
				Value: properties.AppID,
			},
		},
	}, nil
}
