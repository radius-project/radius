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
	"github.com/project-radius/radius/pkg/rp"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface) (rp.RendererOutput, error) {
	resource, ok := dm.(datamodel.DaprInvokeHttpRoute)
	if !ok {
		return rp.RendererOutput{}, conv.ErrInvalidModelConversion
	}
	properties := resource.Properties
	return rp.RendererOutput{
		ComputedValues: map[string]rp.ComputedValueReference{
			"appId": {
				Value: to.String(&properties.AppId),
			},
		},
	}, nil
}
