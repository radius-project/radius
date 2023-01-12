// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volume

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	azvolrenderer "github.com/project-radius/radius/pkg/corerp/renderers/volume/azure"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ renderers.Renderer = (*Renderer)(nil)

// Renderer represents the volume resource renderer.
type Renderer struct {
	VolumeRenderers map[string]VolumeRenderer
}

// NewRenderer creates new renderer for volume resources.
func NewRenderer(armConfig *armauth.ArmConfig) renderers.Renderer {
	return &Renderer{
		VolumeRenderers: map[string]VolumeRenderer{
			datamodel.AzureKeyVaultVolume: &azvolrenderer.KeyVaultRenderer{},
		},
	}
}

// GetDependencyIDs fetches the dependent resources of the volume resource.
func (r *Renderer) GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

// Render renders volume resources to the target platform resource.
func (r *Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.VolumeResource)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	properties := resource.Properties
	if _, ok := r.VolumeRenderers[properties.Kind]; !ok {
		return renderers.RendererOutput{}, fmt.Errorf("%v is not supported", properties.Kind)
	}

	renderOutput, err := r.VolumeRenderers[properties.Kind].Render(ctx, dm, &options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return *renderOutput, nil
}
