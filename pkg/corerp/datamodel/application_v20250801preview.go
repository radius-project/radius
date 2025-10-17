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

const ApplicationResourceType_v20250801preview = "Radius.Core/applications"

var _ v1.DataModelInterface = (*Application_v20250801preview)(nil)

// Application_v20250801preview represents Radius.Core/applications resource.
type Application_v20250801preview struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ApplicationProperties_v20250801preview `json:"properties"`
}

// ResourceTypeName returns the resource type name of the Application instance.
func (e *Application_v20250801preview) ResourceTypeName() string {
	return ApplicationResourceType_v20250801preview
}

// ApplyDeploymentOutput updates the status of the application with the output resources from the deployment and returns no error.
func (c *Application_v20250801preview) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	c.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the OutputResources from the Application resource.
func (c *Application_v20250801preview) OutputResources() []rpv1.OutputResource {
	return c.Properties.Status.OutputResources
}

// ResourceMetadata returns an adapter that provides standardized access to BasicResourceProperties of the Application resource.
func (h *Application_v20250801preview) ResourceMetadata() rpv1.BasicResourcePropertiesAdapter {
	return &h.Properties.BasicResourceProperties
}

// ApplicationProperties_v20250801preview represents the properties of Application resource.
type ApplicationProperties_v20250801preview struct {
	rpv1.BasicResourceProperties
}
