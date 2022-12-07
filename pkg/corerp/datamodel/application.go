// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

var _ conv.DataModelInterface = (*Application)(nil)

// Application represents Application resource.
type Application struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ApplicationProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (e *Application) ResourceTypeName() string {
	return "Applications.Core/applications"
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (c *Application) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	c.Properties.Status.OutputResources = do.DeployedOutputResources
}

// OutputResources returns the output resources array.
func (c *Application) OutputResources() []outputresource.OutputResource {
	return c.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (h *Application) ResourceMetadata() *rp.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// ApplicationProperties represents the properties of Application.
type ApplicationProperties struct {
	rp.BasicResourceProperties

	Extensions []Extension `json:"extensions,omitempty"`
}
