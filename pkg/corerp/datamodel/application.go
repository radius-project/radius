// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

const ApplicationResourceType = "Applications.Core/applications"

var _ v1.DataModelInterface = (*Application)(nil)

// Application represents Application resource.
type Application struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ApplicationProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (e *Application) ResourceTypeName() string {
	return ApplicationResourceType
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (c *Application) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	c.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (c *Application) OutputResources() []rpv1.OutputResource {
	return c.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (h *Application) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// ApplicationProperties represents the properties of Application.
type ApplicationProperties struct {
	rpv1.BasicResourceProperties

	Extensions []Extension `json:"extensions,omitempty"`
}
