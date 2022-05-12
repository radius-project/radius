// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
)

// OperationStatus represents an OperationStatus resource.
type OperationStatus struct {
	TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`

	// Properties is the properties of the resource.
	Properties OperationStatusProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	InternalMetadata
}

func (os OperationStatus) ResourceTypeName() string {
	return "Applications.Core/operationStatuses"
}

// OperationStatusProperties represents the properties of an OperationStatus resource.
type OperationStatusProperties struct {
	Id              string             `json:"id,omitempty"`
	Name            string             `json:"name,omitempty"`
	Status          ProvisioningStates `json:"status,omitempty"`
	StartTime       string             `json:"startTime,omitempty"`
	EndTime         string             `json:"endTime,omitempty"`
	PercentComplete string             `json:"percentComplete,omitempty"`
	Error           error              `json:"error,omitempty"`
}
