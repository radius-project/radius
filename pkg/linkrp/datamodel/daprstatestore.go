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

// DaprStateStore represents DaprStateStore link resource.
type DaprStateStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprStateStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *DaprStateStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues
	if componentName, ok := do.ComputedValues[linkrp.ComponentNameKey].(string); ok {
		r.Properties.ComponentName = componentName
	}
	return nil
}

// OutputResources returns the output resources array.
func (r *DaprStateStore) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *DaprStateStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ComputedValues returns the computed values on the link.
func (r *DaprStateStore) GetComputedValues() map[string]any {
	return r.LinkMetadata.ComputedValues
}

// SecretValues returns the secret values for the link.
func (r *DaprStateStore) GetSecretValues() map[string]rpv1.SecretValueReference {
	return r.LinkMetadata.SecretValues
}

// RecipeData returns the recipe data for the link.
func (r *DaprStateStore) GetRecipeData() linkrp.RecipeData {
	return r.LinkMetadata.RecipeData
}

func (daprStateStore *DaprStateStore) ResourceTypeName() string {
	return linkrp.DaprStateStoresResourceType
}

// DaprStateStoreProperties represents the properties of DaprStateStore resource.
type DaprStateStoreProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties
	Mode     LinkMode          `json:"mode,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
	Recipe   linkrp.LinkRecipe `json:"recipe,omitempty"`
	Resource string            `json:"resource,omitempty"`
	Type     string            `json:"type,omitempty"`
	Version  string            `json:"version,omitempty"`
}
