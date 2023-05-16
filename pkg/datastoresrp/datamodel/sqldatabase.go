// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	linkrpdm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// SqlDatabase represents SqlDatabase link resource.
type SqlDatabase struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties SqlDatabaseProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	linkrpdm.LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *SqlDatabase) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (r *SqlDatabase) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *SqlDatabase) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (sql *SqlDatabase) ResourceTypeName() string {
	return linkrp.N_SqlDatabasesResourceType
}

// SqlDatabaseProperties represents the properties of SqlDatabase resource.
type SqlDatabaseProperties struct {
	rpv1.BasicResourceProperties
	Recipe   linkrp.LinkRecipe `json:"recipe,omitempty"`
	Resource string            `json:"resource,omitempty"`
	Database string            `json:"database,omitempty"`
	Server   string            `json:"server,omitempty"`
	Mode     linkrpdm.LinkMode `json:"mode,omitempty"`
}
