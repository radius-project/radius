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
	linkrp_dm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// RabbitMQQueue represents RabbitMQQueue link resource.
type RabbitMQQueue struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RabbitMQQueueProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	linkrp_dm.LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *RabbitMQQueue) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (r *RabbitMQQueue) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *RabbitMQQueue) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (rabbitmq *RabbitMQQueue) ResourceTypeName() string {
	return linkrp.N_RabbitMQQueuesResourceType
}

// RabbitMQQueueProperties represents the properties of RabbitMQQueue response resource.
type RabbitMQQueueProperties struct {
	rpv1.BasicResourceProperties
	Queue                string                      `json:"queue,omitempty"`
	Recipe               linkrp.LinkRecipe           `json:"recipe,omitempty"`
	Secrets              RabbitMQSecrets             `json:"secrets,omitempty"`
	ResourceProvisioning linkrp.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RabbitMQSecrets struct {
	ConnectionString string `json:"connectionString"`
}

func (rabbitmq RabbitMQSecrets) ResourceTypeName() string {
	return linkrp.N_RabbitMQQueuesResourceType
}

// Recipe returns the recipe for the RabbitMQQueue
func (r *RabbitMQQueue) Recipe() *linkrp.LinkRecipe {
	if r.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &r.Properties.Recipe
}

func (rabbitmq *RabbitMQQueue) VerifyInputs() error {
	properties := rabbitmq.Properties
	if properties.ResourceProvisioning != "" && properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		if properties.Queue == "" {
			return &v1.ErrClientRP{Code: "Bad Request", Message: fmt.Sprintf("queue is required when resourceProvisioning is %s", linkrp.ResourceProvisioningManual)}
		}
	}
	return nil
}
