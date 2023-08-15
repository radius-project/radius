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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

const ExtenderResourceType = "Applications.Core/extenders"

// Extender represents Extender link resource.
type Extender struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ExtenderProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// # Function Explanation
//
// ApplyDeploymentOutput updates the status of the extender instance with the output resources from the
// deployment output.
func (r *Extender) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// # Function Explanation
//
// OutputResources returns the OutputResources from the Extender instance.
func (r *Extender) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// # Function Explanation
//
// ResourceMetadata returns the BasicResourceProperties of the Extender instance.
func (r *Extender) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// # Function Explanation
//
// ResourceTypeName returns the resource type of the extender.
func (extender *Extender) ResourceTypeName() string {
	return ExtenderResourceType
}

// ExtenderProperties represents the properties of Extender resource.
type ExtenderProperties struct {
	rpv1.BasicResourceProperties
	AdditionalProperties map[string]any `json:"additionalProperties,omitempty"`
	Secrets              map[string]any `json:"secrets,omitempty"`
}
