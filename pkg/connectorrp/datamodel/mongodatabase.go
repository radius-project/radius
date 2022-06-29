// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
)

// MongoDatabase represents MongoDatabase connector resource.
type MongoDatabase struct {
	v1.TrackedResource

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties MongoDatabaseProperties `json:"properties"`

	// Any resource values that will be needed for more operations. For example database name to generate secrets for cosmos DB
	ComputedValues map[string]interface{} `json:"computedValues,omitempty"`

	// Stores action to retrieve secret values. For Azure, connectionstring is accessed through cosmos listConnectionString operation, if secrets are not provided as input
	SecretValues map[string]renderers.SecretValueReference `json:"secretValues,omitempty"`
}

type MongoDatabaseResponse struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties MongoDatabaseResponseProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

func (mongo MongoDatabase) ResourceTypeName() string {
	return "Applications.Connector/mongoDatabases"
}

func (mongo MongoDatabaseResponse) ResourceTypeName() string {
	return "Applications.Connector/mongoDatabases"
}

// MongoDatabaseProperties represents the properties of MongoDatabase resource.
type MongoDatabaseResponseProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Environment       string               `json:"environment"`
	Application       string               `json:"application,omitempty"`
	Resource          string               `json:"resource,omitempty"`
	Host              string               `json:"host,omitempty"`
	Port              int32                `json:"port,omitempty"`
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
	return "Applications.Connector/mongoDatabases"
}
