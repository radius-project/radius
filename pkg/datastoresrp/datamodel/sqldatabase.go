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
	"fmt"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/portableresources"
	pr_dm "github.com/radius-project/radius/pkg/portableresources/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

// Recipe returns the ResourceRecipe associated with the SQL database instance if the ResourceProvisioning is not
// set to Manual, otherwise it returns nil.
func (sql *SqlDatabase) Recipe() *portableresources.ResourceRecipe {
	if sql.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		return nil
	}
	return &sql.Properties.Recipe
}

// SqlDatabase represents SQL database portable resource.
type SqlDatabase struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties SqlDatabaseProperties `json:"properties"`

	// ResourceMetadata represents internal DataModel properties common to all portable resources.
	pr_dm.PortableResourceMetadata
}

// ApplyDeploymentOutput updates the output resources of a SQL database resource with the output resources of a DeploymentOutput
// object and returns no error.
func (r *SqlDatabase) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns the OutputResources of the SQL database resource.
func (r *SqlDatabase) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the SQL database resource.
func (r *SqlDatabase) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ResourceTypeName returns the resource type of the SQL database resource.
func (r *SqlDatabase) ResourceTypeName() string {
	return portableresources.SqlDatabasesResourceType
}

// SqlDatabaseProperties represents the properties of SQL database resource.
type SqlDatabaseProperties struct {
	rpv1.BasicResourceProperties
	// The recipe used to automatically deploy underlying infrastructure for the SQL database resource
	Recipe portableresources.ResourceRecipe `json:"recipe,omitempty"`
	// Database name of the target SQL database resource
	Database string `json:"database,omitempty"`
	// The fully qualified domain name of the SQL database resource
	Server string `json:"server,omitempty"`
	// Port value of the target SQL database resource
	Port int32 `json:"port,omitempty"`
	// Specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning portableresources.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
	// List of the resource IDs that support the SQL database resource
	Resources []*portableresources.ResourceReference `json:"resources,omitempty"`
	// Username of the SQL database resource
	Username string `json:"username,omitempty"`
	// Secrets values provided for the resource
	Secrets SqlDatabaseSecrets `json:"secrets,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type SqlDatabaseSecrets struct {
	Password         string `json:"password"`
	ConnectionString string `json:"connectionString"`
}

// VerifyInputs checks that the inputs for manual resource provisioning are all provided
//

// VerifyInputs checks if the required fields are set when the resourceProvisioning is set to manual and returns an error
// if any of the required fields are not set.
func (sql *SqlDatabase) VerifyInputs() error {
	msgs := []string{}
	if sql.Properties.ResourceProvisioning != "" && sql.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		if sql.Properties.Server == "" {
			msgs = append(msgs, "server must be specified when resourceProvisioning is set to manual")
		}
		if sql.Properties.Port == 0 {
			msgs = append(msgs, "port must be specified when resourceProvisioning is set to manual")
		}
		if sql.Properties.Database == "" {
			msgs = append(msgs, "database must be specified when resourceProvisioning is set to manual")
		}
	}

	if len(msgs) == 1 {
		return &v1.ErrClientRP{
			Code:    v1.CodeInvalid,
			Message: msgs[0],
		}
	} else if len(msgs) > 1 {
		return &v1.ErrClientRP{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("multiple errors were found:\n\t%v", strings.Join(msgs, "\n\t")),
		}
	}

	return nil
}

// IsEmpty checks if the SqlDatabaseSecrets struct is empty.
func (sqlSecrets SqlDatabaseSecrets) IsEmpty() bool {
	return sqlSecrets == SqlDatabaseSecrets{}
}

// ResourceTypeName returns the resource type of the SQL database resource.
func (sqlSecrets *SqlDatabaseSecrets) ResourceTypeName() string {
	return portableresources.SqlDatabasesResourceType
}
