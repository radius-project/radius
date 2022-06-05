// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// Application represents Application resource.
type Application struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties ApplicationProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	InternalMetadata v1.InternalMetadata `json:"internalMetadata"`
}

func (e Application) ResourceTypeName() string {
	return "Applications.Core/applications"
}

// ApplicationProperties represents the properties of Application.
type ApplicationProperties struct {
	ProvisioningState v1.ProvisioningStates `json:"provisioningState,omitempty"`
	Environment       string                `json:"environment"`
}
