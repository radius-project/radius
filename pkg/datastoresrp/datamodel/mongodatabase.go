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
	"github.com/project-radius/radius/pkg/linkrp"
	linkrpdm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// MongoDatabase represents MongoDatabase link resource.
type MongoDatabase struct {
	v1.BaseResource

	// LinkMetadata represents internal DataModel properties common to all link types.
	linkrpdm.LinkMetadata

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
	Mode    linkrpdm.LinkMode    `json:"mode"`
}

// Secrets values consisting of secrets provided for the resource
type MongoDatabaseSecrets struct {
	Username         string `json:"username"`
	Password         string `json:"password"`
	ConnectionString string `json:"connectionString"`
}

// # Function Explanation
//
// IsEmpty checks if the MongoDatabaseSecrets instance is empty.
func (mongoSecrets MongoDatabaseSecrets) IsEmpty() bool {
	return mongoSecrets == MongoDatabaseSecrets{}
}

// # Function Explanation
//
// ApplyDeploymentOutput updates the MongoDatabase instance's properties, computed values and secret values
// with the given DeploymentOutput.
func (r *MongoDatabase) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues
	if database, ok := do.ComputedValues[renderers.DatabaseNameValue].(string); ok {
		r.Properties.Database = database
	}

	return nil
}

// # Function Explanation
//
// OutputResources returns the OutputResources from the Status of the MongoDatabase instance.
func (r *MongoDatabase) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// # Function Explanation
//
// ResourceMetadata returns the application resource metadata.
func (r *MongoDatabase) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// # Function Explanation
//
// ResourceTypeName returns the resource type for MongoDatabase.
func (mongoSecrets *MongoDatabaseSecrets) ResourceTypeName() string {
	return linkrp.N_MongoDatabasesResourceType
}

// # Function Explanation
//
// ResourceTypeName returns the resource type for MongoDatabase.
func (mongo *MongoDatabase) ResourceTypeName() string {
	return linkrp.N_MongoDatabasesResourceType
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
