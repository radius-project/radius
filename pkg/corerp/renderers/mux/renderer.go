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

package mux

import (
	"context"
	"errors"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

type Renderer struct {
	Inners map[rpv1.EnvironmentComputeKind]renderers.Renderer
}

// GetDependencyIDs gets the IDs of the dependencies of the given resource.
func (r *Renderer) GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	// Support only kubenretes compute kind for now
	return r.Inners[rpv1.KubernetesComputeKind].GetDependencyIDs(ctx, resource)
}

// Render checks if the DataModelInterface is a ContainerResource and if so, checks for ManualScaling
// extensions and sets the replicas accordingly.
func (r *Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (output renderers.RendererOutput, err error) {
	c := options.Environment.Compute

	inner := r.Inners[rpv1.KubernetesComputeKind]
	output = renderers.RendererOutput{}

	if c != nil {
		switch c.Kind {
		case rpv1.KubernetesComputeKind, rpv1.ACIComputeKind:
			inner = r.Inners[c.Kind]
		default:
			err = errors.New("unsupported compute kind")
			return
		}
	}

	output, err = inner.Render(ctx, dm, options)
	if err != nil {
		output = renderers.RendererOutput{}
	}

	return
}
