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

package datamodel

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

const ApplicationResourceType = "Applications.Core/applications"

var _ v1.DataModelInterface = (*Application)(nil)

// Application represents Application resource.
type Application struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ApplicationProperties `json:"properties"`
}

// ResourceTypeName returns the resource type name of the Application instance.
func (e *Application) ResourceTypeName() string {
	return ApplicationResourceType
}

// ApplyDeploymentOutput updates the status of the application with the output resources from the deployment and returns no error.
func (c *Application) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	c.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the OutputResources from the Application instance.
func (c *Application) OutputResources() []rpv1.OutputResource {
	return c.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the Application instance.
func (h *Application) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// ApplicationProperties represents the properties of Application.
type ApplicationProperties struct {
	rpv1.BasicResourceProperties

	Extensions []Extension `json:"extensions,omitempty"`
}
