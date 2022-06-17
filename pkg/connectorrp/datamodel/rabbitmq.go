// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// RabbitMQMessageQueue represents RabbitMQMessageQueue connector resource.
type RabbitMQMessageQueue struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties RabbitMQMessageQueueProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

func (rabbitmq RabbitMQMessageQueue) ResourceTypeName() string {
	return "Applications.Connector/rabbitMQMessageQueues"
}

// RabbitMQMessageQueueProperties represents the properties of RabbitMQMessageQueue resource.
type RabbitMQMessageQueueProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Environment       string               `json:"environment"`
	Application       string               `json:"application,omitempty"`
	Queue             string               `json:"queue"`
	Secrets           RabbitMQSecrets      `json:"secrets,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RabbitMQSecrets struct {
	ConnectionString string `json:"connectionString"`
}

func (rabbitmq RabbitMQSecrets) ResourceTypeName() string {
	return "Applications.Connector/rabbitMQMessageQueues"
}
