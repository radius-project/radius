// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

// DaprInvokeHttpRoute represents DaprInvokeHttpRoute link resource.
type DaprInvokeHttpRoute struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprInvokeHttpRouteProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *DaprInvokeHttpRoute) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
}

// OutputResources returns the output resources array.
func (r *DaprInvokeHttpRoute) OutputResources() []outputresource.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *DaprInvokeHttpRoute) ResourceMetadata() *rp.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (httpRoute *DaprInvokeHttpRoute) ResourceTypeName() string {
	return linkrp.DaprInvokeHttpRoutesResourceType
}

// DaprInvokeHttpRouteProperties represents the properties of DaprInvokeHttpRoute resource.
type DaprInvokeHttpRouteProperties struct {
	rp.BasicResourceProperties
	Recipe LinkRecipe `json:"recipe,omitempty"`
	AppId  string     `json:"appId"`
}
