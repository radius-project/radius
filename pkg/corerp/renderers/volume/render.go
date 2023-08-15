/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

// # Function Explanation
//
// NewRenderer creates a new Renderer instance with the given ArmConfig, for volume resources.
func NewRenderer(armConfig *armauth.ArmConfig) renderers.Renderer {
	return &Renderer{
		VolumeRenderers: map[string]VolumeRenderer{
			datamodel.AzureKeyVaultVolume: &azvolrenderer.KeyVaultRenderer{},
		},
	}
}

// # Function Explanation
//
// GetDependencyIDs returns nil for the resourceIDs, radiusResourceIDs and an error.
func (r *Renderer) GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

// # Function Explanation
//
// Render checks if the given DataModelInterface is a VolumeResource, and if so, checks if the VolumeRenderers map
// contains a renderer for the VolumeResource's Kind. If so, it calls the renderer and returns the output, otherwise it
// returns an error.
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
