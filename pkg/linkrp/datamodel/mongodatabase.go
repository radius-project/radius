// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

type MongoDatabaseMode string

const (
	MongoDatabaseModeRecipe   MongoDatabaseMode = "recipe"
	MongoDatabaseModeResource MongoDatabaseMode = "resource"
	MongoDatabaseModeValues   MongoDatabaseMode = "values"
)

type MongoDatabase struct {
	v1.BaseResource

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata

	// Properties is the properties of the resource.
	Properties MongoDatabaseProperties `json:"properties"`
}

type MongoDatabaseResponse struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties MongoDatabaseResponseProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// MongoDatabaseProperties represents the properties of MongoDatabase resource.
type MongoDatabaseResponseProperties struct {
	rp.BasicResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Resource          string               `json:"resource,omitempty"`
	Host              string               `json:"host,omitempty"`
	Port              int32                `json:"port,omitempty"`
	Database          string               `json:"database,omitempty"`
	Recipe            LinkRecipe           `json:"recipe,omitempty"`
}

// MongoDatabaseProperties represents the properties of MongoDatabase resource.
type MongoDatabaseProperties struct {
	rp.BasicResourceProperties
	ResourceMongoDatabaseProperties
	RecipeMongoDatabaseProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Mode              MongoDatabaseMode    `json:"mode"`
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

func (mongo MongoDatabase) ResourceTypeName() string {
	return "Applications.Connector/mongoDatabases"
}

func (mongo MongoDatabaseResponse) ResourceTypeName() string {
	return "Applications.Connector/mongoDatabases"
}

type ValuesMongoDatabaseProperties struct {
	Secrets  MongoDatabaseSecrets `json:"secrets,omitempty"`
	Host     string               `json:"host,omitempty"`
	Port     int32                `json:"port,omitempty"`
	Database string               `json:"database,omitempty"`
}

type ResourceMongoDatabaseProperties struct {
	ValuesMongoDatabaseProperties
	Resource string `json:"resource,omitempty"`
}

type RecipeMongoDatabaseProperties struct {
	Recipe LinkRecipe `json:"recipe,omitempty"`
}
