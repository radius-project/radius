// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volume

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ renderers.Renderer = (*Renderer)(nil)

// Renderer represents the volume resource renderer.
type Renderer struct {
	VolumeRenderers map[string]VolumeRenderer
}

// NewRenderer creates new renderer for volume resources.
func NewRenderer() renderers.Renderer {
	return &Renderer{
		VolumeRenderers: map[string]VolumeRenderer{
			datamodel.AzureKeyVaultVolume: &AzureKeyvaultVolumeRenderer{},
		},
	}
}

// GetDependencyIDs fetches the dependent resources of the volume resource.
func (r *Renderer) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

// Render renders volume resources to the target platform resource.
func (r *Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.VolumeResource)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	properties := resource.Properties
	if _, ok := r.VolumeRenderers[properties.Kind]; !ok {
		return renderers.RendererOutput{}, fmt.Errorf("%v is not supported", properties.Kind)
	}

	renderOutput, err := r.VolumeRenderers[properties.Kind].Render(ctx, dm, &options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{
		Resources:      renderOutput.Resources,
		ComputedValues: nil,
		SecretValues:   nil,
	}, nil
}
