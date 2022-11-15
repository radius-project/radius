// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

var _ conv.DataModelInterface = (*Application)(nil)

// Application represents Application resource.
type Application struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ApplicationProperties `json:"properties"`
}

func (e *Application) ResourceTypeName() string {
	return "Applications.Core/applications"
}

// ApplicationProperties represents the properties of Application.
type ApplicationProperties struct {
	rp.BasicResourceProperties
	Extensions []Extension `json:"extensions,omitempty"`
}

// ApplicationKubernetesMetadataExtension - Specifies user defined labels and annotations
type ApplicationKubernetesMetadataExtension struct {
	BaseKubernetesMetadataExtension
}
