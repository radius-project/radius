// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

// MongoDatabase represents MongoDatabase connector resource.
type MongoDatabase struct {
	v1.BaseResource

	// ConnectorMetadata represents internal DataModel properties common to all connector types.
	ConnectorMetadata

	// Properties is the properties of the resource.
	Properties MongoDatabaseProperties `json:"properties"`
}

type MongoDatabaseResponse struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties MongoDatabaseResponseProperties `json:"properties"`

	// ConnectorMetadata represents internal DataModel properties common to all connector types.
	ConnectorMetadata
}

func (mongo MongoDatabase) ResourceTypeName() string {
	return "Applications.Link/mongoDatabases"
}

func (mongo MongoDatabaseResponse) ResourceTypeName() string {
	return "Applications.Link/mongoDatabases"
}

// MongoDatabaseProperties represents the properties of MongoDatabase resource.
type MongoDatabaseResponseProperties struct {
	rp.BasicResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Resource          string               `json:"resource,omitempty"`
	Host              string               `json:"host,omitempty"`
	Port              int32                `json:"port,omitempty"`
	Database          string               `json:"database,omitempty"`
	Recipe            ConnectorRecipe      `json:"recipe,omitempty"`
}

type MongoDatabaseProperties struct {
	MongoDatabaseResponseProperties
	Secrets MongoDatabaseSecrets `json:"secrets,omitempty"`
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

func (mongoSecrets MongoDatabaseSecrets) ResourceTypeName() string {
	return "Applications.Link/mongoDatabases"
}
