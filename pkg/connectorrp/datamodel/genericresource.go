// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// GenericResourceVersionAgnostic represents Generic resource
type GenericResourceVersionAgnostic struct {
	v1.TrackedResource
	// Any object
	ResourceProperties map[string]interface{} `json:"resourceProperties,omitempty"`

	// READ-ONLY; Metadata pertaining to creation and last modification of the resource.
	SystemData v1.SystemData `json:"systemData,omitempty" azure:"ro"`
}

func (gr GenericResourceVersionAgnostic) ResourceTypeName() string {
	return "Generic Resource"
}
