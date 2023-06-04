/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// Recipe returns the recipe for the SqlDatabase
func (sql *SqlDatabase) Recipe() *linkrp.LinkRecipe {
	if sql.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &sql.Properties.Recipe
}

// SqlDatabase represents SqlDatabase link resource.
type SqlDatabase struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties SqlDatabaseProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
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
	return linkrp.SqlDatabasesResourceType
}

// SqlDatabaseProperties represents the properties of SqlDatabase resource.
type SqlDatabaseProperties struct {
	rpv1.BasicResourceProperties
	Recipe               linkrp.LinkRecipe           `json:"recipe,omitempty"`
	Database             string                      `json:"database,omitempty"`
	Server               string                      `json:"server,omitempty"`
	ResourceProvisioning linkrp.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
	Resources            []*linkrp.ResourceReference `json:"resources,omitempty"`
}
