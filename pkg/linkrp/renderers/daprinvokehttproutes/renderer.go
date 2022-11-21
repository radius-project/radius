// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprinvokehttproutes

import (
	"context"
	"errors"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/rp"
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
	_, err := renderers.ValidateApplicationID(properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	return renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"appId": {
				Value: to.String(&properties.AppId),
				Transformer: func(r conv.DataModelInterface, cv map[string]any) error {
					appId, ok := cv[AppIDKey].(string)
					if !ok {
						return errors.New("app id must be set on computed values for DaprInvokeHttpRoute")
					}
					res, ok := r.(*datamodel.DaprInvokeHttpRoute)
					if !ok {
						return errors.New("resource must be DaprInvokeHttpRoute")
					}

					res.Properties.AppId = appId
					return nil
				},
			},
		},
		SecretValues: map[string]rp.SecretValueReference{},
	}, nil
}
