// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprinvokehttproutes

import (
	"context"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.DaprInvokeHttpRoute)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}
	properties := resource.Properties
	return renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"appId": {
				Value: to.String(&properties.AppId),
			},
		},
	}, nil
}
