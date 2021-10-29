// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha3

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/renderers"
)

type KubernetesRenderer struct {
}

// Render is the WorkloadRenderer implementation for volume
func (r *KubernetesRenderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := VolumeProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	return renderers.RendererOutput{}, fmt.Errorf("Kind %s is not supported.", properties.Kind)
}
