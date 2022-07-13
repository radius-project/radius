// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

import (
	"context"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/rp"
)

const (
	ResourceType = "Applications.Connector/extenders"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r *Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.Extender)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	properties := resource.Properties

	computedValues, secretValues := MakeSecretsAndValues(resource.Name, properties)

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{},
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func MakeSecretsAndValues(name string, properties datamodel.ExtenderProperties) (map[string]renderers.ComputedValueReference, map[string]rp.SecretValueReference) {
	computedValueReferences := map[string]renderers.ComputedValueReference{}
	for k, v := range properties.AdditionalProperties {
		computedValueReferences[k] = renderers.ComputedValueReference{
			Value: v,
		}
	}
	if properties.Secrets == nil {
		return computedValueReferences, nil
	}

	// Create secret value references to point to the secret output resources created above
	secretValues := map[string]rp.SecretValueReference{}
	for k, v := range properties.Secrets {
		secretValues[k] = rp.SecretValueReference{
			Value: v.(string),
		}
	}

	return computedValueReferences, secretValues
}
