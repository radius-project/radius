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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// RabbitMQMessageQueue represents RabbitMQMessageQueue link resource.
type RabbitMQMessageQueue struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RabbitMQMessageQueueProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// # Function Explanation
//
// ApplyDeploymentOutput updates the output resources of the RabbitMQMessageQueue resource with
// the DeployedOutputResources.
func (r *RabbitMQMessageQueue) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// # Function Explanation
//
// OutputResources returns the OutputResources of the RabbitMQMessageQueue resource.
func (r *RabbitMQMessageQueue) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// # Function Explanation
//
// ResourceMetadata returns the BasicResourceProperties of the RabbitMQMessageQueue resource.
func (r *RabbitMQMessageQueue) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// # Function Explanation
//
// ResourceTypeName returns the resource type for RabbitMQMessageQueue resource.
func (rabbitmq *RabbitMQMessageQueue) ResourceTypeName() string {
	return linkrp.RabbitMQMessageQueuesResourceType
}

// RabbitMQMessageQueueProperties represents the properties of RabbitMQMessageQueue response resource.
type RabbitMQMessageQueueProperties struct {
	rpv1.BasicResourceProperties
	Queue                string                      `json:"queue,omitempty"`
	Host                 string                      `json:"host,omitempty"`
	Port                 int32                       `json:"port,omitempty"`
	VHost                string                      `json:"vHost,omitempty"`
	Username             string                      `json:"username,omitempty"`
	Resources            []*linkrp.ResourceReference `json:"resources,omitempty"`
	Recipe               linkrp.LinkRecipe           `json:"recipe,omitempty"`
	Secrets              RabbitMQSecrets             `json:"secrets,omitempty"`
	ResourceProvisioning linkrp.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
	TLS                  bool                        `json:"tls,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RabbitMQSecrets struct {
	URI      string `json:"uri,omitempty"`
	Password string `json:"password,omitempty"`
}

// # Function Explanation
//
// ResourceTypeName returns the resource type for RabbitMQMessageQueue resource.
func (rabbitmq RabbitMQSecrets) ResourceTypeName() string {
	return linkrp.RabbitMQMessageQueuesResourceType
}

// # Function Explanation
//
// Recipe returns the LinkRecipe associated with the RabbitMQMessageQueue resource, or nil if the
// ResourceProvisioning is set to Manual.
func (r *RabbitMQMessageQueue) Recipe() *linkrp.LinkRecipe {
	if r.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &r.Properties.Recipe
}

// # Function Explanation
//
// VerifyInputs checks if the required fields are present in the RabbitMQMessageQueue instance and returns an error if not.
func (rabbitmq *RabbitMQMessageQueue) VerifyInputs() error {
	properties := rabbitmq.Properties
	if properties.ResourceProvisioning != "" && properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		if properties.Queue == "" {
			return &v1.ErrClientRP{Code: "Bad Request", Message: fmt.Sprintf("queue is required when resourceProvisioning is %s", linkrp.ResourceProvisioningManual)}
		}
	}
	return nil
}
