// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

// DaprInvokeHttpRoute represents DaprInvokeHttpRoute link resource.
type DaprInvokeHttpRoute struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprInvokeHttpRouteProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (httpRoute DaprInvokeHttpRoute) ResourceTypeName() string {
	return "Applications.Link/daprInvokeHttpRoutes"
}

// DaprInvokeHttpRouteProperties represents the properties of DaprInvokeHttpRoute resource.
type DaprInvokeHttpRouteProperties struct {
	rp.BasicResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Recipe            LinkRecipe           `json:"recipe,omitempty"`
	AppId             string               `json:"appId"`
}
