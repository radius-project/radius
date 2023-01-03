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
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ExtenderProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (extender Extender) ResourceTypeName() string {
	return "Applications.Link/extenders"
}

// ExtenderProperties represents the properties of Extender resource.
type ExtenderProperties struct {
	rp.BasicResourceProperties
	AdditionalProperties map[string]any       `json:"additionalProperties,omitempty"`
	ProvisioningState    v1.ProvisioningState `json:"provisioningState,omitempty"`
	Secrets              map[string]any       `json:"secrets,omitempty"`
}
