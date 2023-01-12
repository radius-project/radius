// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volume

import (
	"context"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/renderers"
)

const (
	ResourceType = "Applications.Core/volumes"
)

// VolumeRenderer is the interface representing Volume resource.
type VolumeRenderer interface {
	Render(context.Context, v1.DataModelInterface, *renderers.RenderOptions) (*renderers.RendererOutput, error)
}
