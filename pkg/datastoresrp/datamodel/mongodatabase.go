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

// MongoDatabase represents Mongo database portable resource.
type MongoDatabase struct {
	v1.BaseResource

	// PortableResourceMetadata represents internal DataModel properties common to all portable resources.
	pr_dm.PortableResourceMetadata

	// Properties is the properties of the resource.
	Properties MongoDatabaseProperties `json:"properties"`
}

// MongoDatabaseProperties represents the properties of Mongo database resource.
type MongoDatabaseProperties struct {
	rpv1.BasicResourceProperties
	// Secrets values provided for the Mongo database resource
	Secrets MongoDatabaseSecrets `json:"secrets,omitempty"`
	// Host name of the target Mongo database
	Host string `json:"host,omitempty"`
	// Port value of the target Mongo database
	Port int32 `json:"port,omitempty"`
	// Database name of the target Mongo database
	Database string `json:"database,omitempty"`
	// The recipe used to automatically deploy underlying infrastructure for the Mongo database link
	Recipe portableresources.ResourceRecipe `json:"recipe,omitempty"`
	// List of the resource IDs that support the Mongo database resource
	Resources []*portableresources.ResourceReference `json:"resources,omitempty"`
	// Specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning portableresources.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
	// Username of the Mongo database
	Username string `json:"username,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type MongoDatabaseSecrets struct {
	Password         string `json:"password"`
	ConnectionString string `json:"connectionString"`
}

// IsEmpty checks if the MongoDatabaseSecrets instance is empty.
func (mongoSecrets MongoDatabaseSecrets) IsEmpty() bool {
	return mongoSecrets == MongoDatabaseSecrets{}
}

// VerifyInputs checks if the manual resource provisioning fields are set and returns an error if any of them are missing.
func (r *MongoDatabase) VerifyInputs() error {
	msgs := []string{}
	if r.Properties.ResourceProvisioning != "" && r.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		if r.Properties.Host == "" {
			msgs = append(msgs, "host must be specified when resourceProvisioning is set to manual")
		}
		if r.Properties.Port == 0 {
			msgs = append(msgs, "port must be specified when resourceProvisioning is set to manual")
		}
		if r.Properties.Database == "" {
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

// ApplyDeploymentOutput updates the Mongo database instance's database property, output resources, computed values
// and secret values with the given DeploymentOutput.
func (r *MongoDatabase) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns the OutputResources of the Mongo database instance.
func (r *MongoDatabase) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the Mongo database instance i.e. application resource metadata.
func (r *MongoDatabase) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// Recipe returns the ResourceRecipe associated with the Mongo database instance, or nil if the
// ResourceProvisioning is set to Manual.
func (r *MongoDatabase) Recipe() *portableresources.ResourceRecipe {
	if r.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		return nil
	}
	return &r.Properties.Recipe
}

// ResourceTypeName returns the resource type for Mongo database resource.
func (mongoSecrets *MongoDatabaseSecrets) ResourceTypeName() string {
	return portableresources.MongoDatabasesResourceType
}

// ResourceTypeName returns the resource type for Mongo database resource.
func (r *MongoDatabase) ResourceTypeName() string {
	return portableresources.MongoDatabasesResourceType
}
