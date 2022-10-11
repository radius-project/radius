// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volume

import (
	"context"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
)

const (
	// ResourceType represents volume resource type name.
	ResourceType = "Applications.Core/volumes"
)

// VolumeRenderer is the interface representing Volume resource.
type VolumeRenderer interface {
	Render(context.Context, conv.DataModelInterface, *renderers.RenderOptions) (renderers.RendererOutput, error)
}

// SecretObjects wraps the different secret objects to be configured on the SecretProvider class
type SecretObjects struct {
	secrets      map[string]datamodel.SecretObjectProperties
	certificates map[string]datamodel.CertificateObjectProperties
	keys         map[string]datamodel.KeyObjectProperties
}

type objectValues struct {
	alias    string
	version  string
	encoding string
	format   string
}
