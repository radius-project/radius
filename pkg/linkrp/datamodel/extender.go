// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

// Extender represents Extender link resource.
type Extender struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties ExtenderProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

type ExtenderResponse struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties ExtenderResponseProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (extender Extender) ResourceTypeName() string {
	return "Applications.Link/extenders"
}

func (extender ExtenderResponse) ResourceTypeName() string {
	return "Applications.Link/extenders"
}

// ExtenderProperties represents the properties of Extender resource.
type ExtenderProperties struct {
	ExtenderResponseProperties
	Secrets map[string]interface{} `json:"secrets,omitempty"`
}

// ExtenderProperties represents the properties of Extender resource.
type ExtenderResponseProperties struct {
	rp.BasicResourceProperties
	AdditionalProperties map[string]interface{} `json:"additionalProperties,omitempty"`
	ProvisioningState    v1.ProvisioningState   `json:"provisioningState,omitempty"`
}
