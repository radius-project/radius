// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

// Extender represents Extender link resource.
type Extender struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ExtenderProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (r *Extender) Transform(outputResources []outputresource.OutputResource, computedValues map[string]any, secretValues map[string]rp.SecretValueReference) error {
	r.Properties.Status.OutputResources = outputResources
	r.ComputedValues = computedValues
	r.SecretValues = secretValues
	return nil
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *Extender) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
}

// OutputResources returns the output resources array.
func (r *Extender) OutputResources() []outputresource.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *Extender) ResourceMetadata() *rp.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ComputedValues returns the computed values on the link.
func (r *Extender) GetComputedValues() map[string]any {
	return r.LinkMetadata.ComputedValues
}

// SecretValues returns the secret values for the link.
func (r *Extender) GetSecretValues() map[string]rp.SecretValueReference {
	return r.LinkMetadata.SecretValues
}

// RecipeData returns the recipe data for the link.
func (r *Extender) GetRecipeData() RecipeData {
	return r.LinkMetadata.RecipeData
}

func (extender *Extender) ResourceTypeName() string {
	return "Applications.Link/extenders"
}

// ExtenderProperties represents the properties of Extender resource.
type ExtenderProperties struct {
	rp.BasicResourceProperties
	AdditionalProperties map[string]any `json:"additionalProperties,omitempty"`
	Secrets              map[string]any `json:"secrets,omitempty"`
}
