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
	InternalMetadata InternalMetadata `json:"internalMetadata"`
}

func (mongo MongoDatabase) ResourceTypeName() string {
	return "Applications.Connector/mongoDatabases"
}

// MongoDatabaseProperties represents the properties of MongoDatabase resource.
type MongoDatabaseProperties struct {
	ProvisioningState ProvisioningStates `json:"provisioningState,omitempty"`
	Application       string             `json:"application,omitempty"`
	FromResource      FromResource       `json:"fromResource,omitempty"`
	FromValues        FromValues         `json:"fromValues,omitempty"`
}

// FromResource represents the target resource that the mongo database connector binds to
type FromResource struct {
	Source string `json:"source"` // Fully qualified resource ID for the resource that the connector binds to
}

// FromValues values provided for the target resource that the mongo database connector binds to
type FromValues struct {
	ConnectionString string `json:"connectionString"`
	Username         string `json:"username"`
	Password         string `json:"password"`
}
