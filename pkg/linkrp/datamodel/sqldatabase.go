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

// SqlDatabase represents SqlDatabase link resource.
type SqlDatabase struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties SqlDatabaseProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *SqlDatabase) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
}

// OutputResources returns the output resources array.
func (r *SqlDatabase) OutputResources() []outputresource.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *SqlDatabase) ResourceMetadata() *rp.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (sql *SqlDatabase) ResourceTypeName() string {
	return linkrp.SqlDatabasesResourceType
}

// SqlDatabaseProperties represents the properties of SqlDatabase resource.
type SqlDatabaseProperties struct {
	rp.BasicResourceProperties
	Recipe   LinkRecipe `json:"recipe,omitempty"`
	Resource string     `json:"resource,omitempty"`
	Database string     `json:"database,omitempty"`
	Server   string     `json:"server,omitempty"`
	Mode     LinkMode   `json:"mode,omitempty"`
}
