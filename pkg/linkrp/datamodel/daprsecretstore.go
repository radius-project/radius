// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// DaprSecretStore represents DaprSecretStore link resource.
type DaprSecretStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprSecretStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *DaprSecretStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues
	if componentName, ok := do.ComputedValues[linkrp.ComponentNameKey].(string); ok {
		r.Properties.ComponentName = componentName
	}
	return nil
}

// OutputResources returns the output resources array.
func (r *DaprSecretStore) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *DaprSecretStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ComputedValues returns the computed values on the link.
func (r *DaprSecretStore) GetComputedValues() map[string]any {
	return r.LinkMetadata.ComputedValues
}

// SecretValues returns the secret values for the link.
func (r *DaprSecretStore) GetSecretValues() map[string]rpv1.SecretValueReference {
	return r.LinkMetadata.SecretValues
}

// RecipeData returns the recipe data for the link.
func (r *DaprSecretStore) GetRecipeData() linkrp.RecipeData {
	return r.LinkMetadata.RecipeData
}

func (daprSecretStore *DaprSecretStore) ResourceTypeName() string {
	return linkrp.DaprSecretStoresResourceType
}

// DaprSecretStoreProperties represents the properties of DaprSecretStore resource.
type DaprSecretStoreProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties
	Mode     LinkMode          `json:"mode"`
	Type     string            `json:"type"`
	Version  string            `json:"version"`
	Metadata map[string]any    `json:"metadata"`
	Recipe   linkrp.LinkRecipe `json:"recipe,omitempty"`
}
