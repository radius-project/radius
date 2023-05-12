// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// Extender represents Extender link resource.
type Extender struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ExtenderProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *Extender) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (r *Extender) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *Extender) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (extender *Extender) ResourceTypeName() string {
	return "Applications.Core/extenders"
}

// ExtenderProperties represents the properties of Extender resource.
type ExtenderProperties struct {
	rpv1.BasicResourceProperties
	AdditionalProperties map[string]any `json:"additionalProperties,omitempty"`
	Secrets              map[string]any `json:"secrets,omitempty"`
}
