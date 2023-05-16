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

// RabbitMQQueue represents RabbitMQQueues portable resource.
type RabbitMQQueue struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RabbitMQQueueProperties `json:"properties"`

	// messagingMetadata represents internal DataModel properties common to all portable resource types.
	linkrpdm.LinkMetadata
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
	Queue   string            `json:"queue"`
	Recipe  linkrp.LinkRecipe `json:"recipe,omitempty"`
	Secrets RabbitMQSecrets   `json:"secrets,omitempty"`
	Mode    linkrpdm.LinkMode `json:"mode,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RabbitMQSecrets struct {
	ConnectionString string `json:"connectionString"`
}

func (rabbitmq RabbitMQSecrets) ResourceTypeName() string {
	return linkrp.N_RabbitMQQueuesResourceType
}
