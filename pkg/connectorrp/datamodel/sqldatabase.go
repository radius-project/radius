// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
)

// SqlDatabase represents SqlDatabase connector resource.
type SqlDatabase struct {
	basedatamodel.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties SqlDatabaseProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	basedatamodel.InternalMetadata
}

func (sql SqlDatabase) ResourceTypeName() string {
	return "Applications.Connector/sqlDatabases"
}

// SqlDatabaseProperties represents the properties of SqlDatabase resource.
type SqlDatabaseProperties struct {
	ProvisioningState basedatamodel.ProvisioningStates `json:"provisioningState,omitempty"`
	Environment       string                           `json:"environment"`
	Application       string                           `json:"application,omitempty"`
	Resource          string                           `json:"resource,omitempty"`
	Database          string                           `json:"database,omitempty"`
	Server            string                           `json:"server,omitempty"`
}
