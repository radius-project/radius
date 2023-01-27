// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// MongoDatabase represents MongoDatabase link resource.
type MongoDatabase struct {
	v1.BaseResource

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata

	// Properties is the properties of the resource.
	Properties MongoDatabaseProperties `json:"properties"`
}

// MongoDatabaseProperties represents the properties of MongoDatabase resource.
type MongoDatabaseProperties struct {
	rpv1.BasicResourceProperties
	MongoDatabaseResourceProperties
	MongoDatabaseRecipeProperties
	MongoDatabaseValuesProperties
	Secrets MongoDatabaseSecrets `json:"secrets,omitempty"`
	Mode    LinkMode             `json:"mode"`
}

// Secrets values consisting of secrets provided for the resource
type MongoDatabaseSecrets struct {
	Username         string `json:"username"`
	Password         string `json:"password"`
	ConnectionString string `json:"connectionString"`
}

func (mongoSecrets MongoDatabaseSecrets) IsEmpty() bool {
	return mongoSecrets == MongoDatabaseSecrets{}
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *MongoDatabase) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues
	if database, ok := do.ComputedValues[renderers.DatabaseNameValue].(string); ok {
		r.Properties.Database = database
	}
	r.RecipeData = do.RecipeData
	return nil
}

// OutputResources returns the output resources array.
func (r *MongoDatabase) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *MongoDatabase) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (mongoSecrets *MongoDatabaseSecrets) ResourceTypeName() string {
	return linkrp.MongoDatabasesResourceType
}

func (mongo *MongoDatabase) ResourceTypeName() string {
	return linkrp.MongoDatabasesResourceType
}

type MongoDatabaseValuesProperties struct {
	Host     string `json:"host,omitempty"`
	Port     int32  `json:"port,omitempty"`
	Database string `json:"database,omitempty"`
}

type MongoDatabaseResourceProperties struct {
	Resource string `json:"resource,omitempty"`
}

type MongoDatabaseRecipeProperties struct {
	Recipe linkrp.LinkRecipe `json:"recipe,omitempty"`
}
