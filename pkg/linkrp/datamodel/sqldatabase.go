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

// SqlDatabase represents SqlDatabase link resource.
type SqlDatabase struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties SqlDatabaseProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (r *SqlDatabase) Transform(outputResources []outputresource.OutputResource, computedValues map[string]any, secretValues map[string]rp.SecretValueReference) error {
	r.Properties.Status.OutputResources = outputResources
	r.ComputedValues = computedValues
	r.SecretValues = secretValues
	if server, ok := computedValues[linkrp.ServerNameValue].(string); ok {
		r.Properties.Server = server
	}
	if database, ok := computedValues[linkrp.DatabaseNameValue].(string); ok {
		r.Properties.Database = database
	}

	return nil
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *SqlDatabase) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (r *SqlDatabase) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *SqlDatabase) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ComputedValues returns the computed values on the link.
func (r *SqlDatabase) GetComputedValues() map[string]any {
	return r.LinkMetadata.ComputedValues
}

// SecretValues returns the secret values for the link.
func (r *SqlDatabase) GetSecretValues() map[string]rp.SecretValueReference {
	return r.LinkMetadata.SecretValues
}

// RecipeData returns the recipe data for the link.
func (r *SqlDatabase) GetRecipeData() RecipeData {
	return r.LinkMetadata.RecipeData
}

func (sql *SqlDatabase) ResourceTypeName() string {
	return linkrp.SqlDatabasesResourceType
}

// SqlDatabaseProperties represents the properties of SqlDatabase resource.
type SqlDatabaseProperties struct {
	rpv1.BasicResourceProperties
	Recipe   linkrp.LinkRecipe `json:"recipe,omitempty"`
	Resource string            `json:"resource,omitempty"`
	Database string            `json:"database,omitempty"`
	Server   string            `json:"server,omitempty"`
	Mode     LinkMode          `json:"mode,omitempty"`
}
