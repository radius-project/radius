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

// RabbitMQQueue represents RabbitMQQueue portable resource.
type RabbitMQQueue struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RabbitMQQueueProperties `json:"properties"`

	// ResourceMetadata represents internal DataModel properties common to all portable resource types.
	pr_dm.PortableResourceMetadata
}

// ApplyDeploymentOutput updates the RabbitMQQueue instance with the DeployedOutputResources from the
// DeploymentOutput object and returns no error.
func (r *RabbitMQQueue) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns the OutputResources from the Properties of the RabbitMQQueue instance.
func (r *RabbitMQQueue) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the RabbitMQQueue instance.
func (r *RabbitMQQueue) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ResourceTypeName returns the resource type name for RabbitMQ queues.
func (r *RabbitMQQueue) ResourceTypeName() string {
	return portableresources.RabbitMQQueuesResourceType
}

// SetDeploymentStatus updates the deployment status of the RabbitMQ resource.
func (r *RabbitMQQueue) SetDeploymentStatus(status portableresources.RecipeDeploymentStatus) {
	r.Recipe().DeploymentStatus = status
}

// RabbitMQQueueProperties represents the properties of RabbitMQQueue response resource.
type RabbitMQQueueProperties struct {
	rpv1.BasicResourceProperties
	Queue                string                                 `json:"queue,omitempty"`
	Host                 string                                 `json:"host,omitempty"`
	Port                 int32                                  `json:"port,omitempty"`
	VHost                string                                 `json:"vHost,omitempty"`
	Username             string                                 `json:"username,omitempty"`
	Resources            []*portableresources.ResourceReference `json:"resources,omitempty"`
	Recipe               portableresources.ResourceRecipe       `json:"recipe,omitempty"`
	Secrets              RabbitMQSecrets                        `json:"secrets,omitempty"`
	ResourceProvisioning portableresources.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
	TLS                  bool                                   `json:"tls,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RabbitMQSecrets struct {
	URI      string `json:"uri,omitempty"`
	Password string `json:"password,omitempty"`
}

// ResourceTypeName returns the resource type name for RabbitMQ queues.
func (rabbitmq RabbitMQSecrets) ResourceTypeName() string {
	return portableresources.RabbitMQQueuesResourceType
}

// Recipe returns the recipe for the RabbitMQQueue. It gets the ResourceRecipe associated with the RabbitMQQueue instance
// if the ResourceProvisioning is not set to Manual, otherwise it returns nil.
func (r *RabbitMQQueue) Recipe() *portableresources.ResourceRecipe {
	if r.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		return nil
	}
	return &r.Properties.Recipe
}

// VerifyInputs checks if the queue is provided when resourceProvisioning is set to manual and returns an error if not.
func (r *RabbitMQQueue) VerifyInputs() error {
	properties := r.Properties
	msgs := []string{}
	if properties.ResourceProvisioning != "" && properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		if properties.Queue == "" {
			return &v1.ErrClientRP{Code: "Bad Request", Message: fmt.Sprintf("queue is required when resourceProvisioning is %s", portableresources.ResourceProvisioningManual)}
		}
		if properties.Host == "" {
			msgs = append(msgs, "host must be specified when resourceProvisioning is set to manual")
		}
		if properties.Port == 0 {
			msgs = append(msgs, "port must be specified when resourceProvisioning is set to manual")
		}
		if properties.Username == "" && properties.Secrets.Password != "" {
			msgs = append(msgs, "username must be provided with password")
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
