// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

// DaprSecretStore represents DaprSecretStore link resource.
type DaprSecretStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprSecretStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (r *DaprSecretStore) Transform(outputResources []outputresource.OutputResource, computedValues map[string]any, secretValues map[string]rp.SecretValueReference) error {
	r.Properties.Status.OutputResources = outputResources
	r.ComputedValues = computedValues
	r.SecretValues = secretValues
	if componentName, ok := computedValues[linkrp.ComponentNameKey].(string); ok {
		r.Properties.ComponentName = componentName
	}

	return nil
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *DaprSecretStore) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
}

// OutputResources returns the output resources array.
func (r *DaprSecretStore) OutputResources() []outputresource.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *DaprSecretStore) ResourceMetadata() *rp.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ComputedValues returns the computed values on the link.
func (r *DaprSecretStore) GetComputedValues() map[string]any {
	return r.LinkMetadata.ComputedValues
}

// SecretValues returns the secret values for the link.
func (r *DaprSecretStore) GetSecretValues() map[string]rp.SecretValueReference {
	return r.LinkMetadata.SecretValues
}

// RecipeData returns the recipe data for the link.
func (r *DaprSecretStore) GetRecipeData() RecipeData {
	return r.LinkMetadata.RecipeData
}

func (daprSecretStore *DaprSecretStore) ResourceTypeName() string {
	return "Applications.Link/daprSecretStores"
}

// DaprSecretStoreProperties represents the properties of DaprSecretStore resource.
type DaprSecretStoreProperties struct {
	rp.BasicResourceProperties
	rp.BasicDaprResourceProperties
	Mode     LinkMode       `json:"mode"`
	Type     string         `json:"type"`
	Version  string         `json:"version"`
	Metadata map[string]any `json:"metadata"`
	Recipe   LinkRecipe     `json:"recipe,omitempty"`
}
