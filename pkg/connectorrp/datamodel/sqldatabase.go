// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// SqlDatabase represents SqlDatabase connector resource.
type SqlDatabase struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties SqlDatabaseProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

func (sql SqlDatabase) ResourceTypeName() string {
	return "Applications.Connector/sqlDatabases"
}

// SqlDatabaseProperties represents the properties of SqlDatabase resource.
type SqlDatabaseProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Resource          string               `json:"resource,omitempty"`
	Database          string               `json:"database,omitempty"`
	Server            string               `json:"server,omitempty"`
}
