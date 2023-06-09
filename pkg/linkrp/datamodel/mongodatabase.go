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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// MongoDatabase represents MongoDatabase link resource.
type MongoDatabase struct {
	v1.BaseResource

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata

	// Properties is the properties of the resource.
	Properties MongoDatabaseProperties `json:"properties"`
}

// MongoDatabaseProperties represents the properties of MongoDatabase resource.
type MongoDatabaseProperties struct {
	rpv1.BasicResourceProperties
	// Secrets values provided for the resource
	Secrets MongoDatabaseSecrets `json:"secrets,omitempty"`
	// Host name of the target Mongo database
	Host string `json:"host,omitempty"`
	// Port value of the target Mongo database
	Port int32 `json:"port,omitempty"`
	// Database name of the target Mongo database
	Database string `json:"database,omitempty"`
	// The recipe used to automatically deploy underlying infrastructure for the MongoDB link
	Recipe linkrp.LinkRecipe `json:"recipe,omitempty"`
	// List of the resource IDs that support the MongoDB resource
	Resources []*linkrp.ResourceReference `json:"resources,omitempty"`
	// Specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning linkrp.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
	// Username of the Mongo database
	Username string `json:"username"`
}

// Secrets values consisting of secrets provided for the resource
type MongoDatabaseSecrets struct {
	Password         string `json:"password"`
	ConnectionString string `json:"connectionString"`
}

func (mongoSecrets MongoDatabaseSecrets) IsEmpty() bool {
	return mongoSecrets == MongoDatabaseSecrets{}
}

// VerifyInputs checks that the inputs for manual resource provisioning are all provided
func (mongodb *MongoDatabase) VerifyInputs() error {
	msgs := []string{}
	if mongodb.Properties.ResourceProvisioning != "" && mongodb.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		if mongodb.Properties.Host == "" {
			msgs = append(msgs, "host must be specified when resourceProvisioning is set to manual")
		}
		if mongodb.Properties.Port == 0 {
			msgs = append(msgs, "port must be specified when resourceProvisioning is set to manual")
		}
		if mongodb.Properties.Database == "" {
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

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *MongoDatabase) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues
	if database, ok := do.ComputedValues[renderers.DatabaseNameValue].(string); ok {
		r.Properties.Database = database
	}

	return nil
}

// OutputResources returns the output resources array.
func (r *MongoDatabase) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *MongoDatabase) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (r *MongoDatabase) Recipe() *linkrp.LinkRecipe {
	if r.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &r.Properties.Recipe
}

func (mongoSecrets *MongoDatabaseSecrets) ResourceTypeName() string {
	return linkrp.MongoDatabasesResourceType
}

func (mongo *MongoDatabase) ResourceTypeName() string {
	return linkrp.MongoDatabasesResourceType
}
