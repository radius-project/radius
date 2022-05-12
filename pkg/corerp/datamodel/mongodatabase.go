// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
)

// MongoDatabase represents MongoDatabase connector resource.
type MongoDatabase struct {
	TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties MongoDatabaseProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	InternalMetadata
}

func (mongo MongoDatabase) ResourceTypeName() string {
	return "Applications.Connector/mongoDatabases"
}

// MongoDatabaseProperties represents the properties of MongoDatabase resource.
type MongoDatabaseProperties struct {
	ProvisioningState ProvisioningStates `json:"provisioningState,omitempty"`
	Environment       string             `json:"environment"`
	Application       string             `json:"application,omitempty"`
	Resource          string             `json:"resource,omitempty"`
	Host              string             `json:"host,omitempty"`
	Port              int                `json:"port,omitempty"`
	Secrets           Secrets            `json:"secrets,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type Secrets struct {
	Username         string `json:"username"`
	Password         string `json:"password"`
	ConnectionString string `json:"connectionString"`
}
