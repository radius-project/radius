// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

type RabbitMQMessageQueuePropertiesMode string

const (
	RabbitMQMessageQueuePropertiesModeRecipe  RabbitMQMessageQueuePropertiesMode = "recipe"
	RabbitMQMessageQueuePropertiesModeValues  RabbitMQMessageQueuePropertiesMode = "values"
	RabbitMQMessageQueuePropertiesModeUnknown RabbitMQMessageQueuePropertiesMode = "unknown"
)

// RabbitMQMessageQueue represents RabbitMQMessageQueue link resource.
type RabbitMQMessageQueue struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties RabbitMQMessageQueueProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (rabbitmq RabbitMQMessageQueue) ResourceTypeName() string {
	return "Applications.Link/rabbitMQMessageQueues"
}

// RabbitMQMessageQueueProperties represents the properties of RabbitMQMessageQueue response resource.
type RabbitMQMessageQueueProperties struct {
	rp.BasicResourceProperties
	ProvisioningState v1.ProvisioningState               `json:"provisioningState,omitempty"`
	Queue             string                             `json:"queue"`
	Recipe            LinkRecipe                         `json:"recipe,omitempty"`
	Secrets           RabbitMQSecrets                    `json:"secrets,omitempty"`
	Mode              RabbitMQMessageQueuePropertiesMode `json:"mode,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RabbitMQSecrets struct {
	ConnectionString string `json:"connectionString"`
}

func (rabbitmq RabbitMQSecrets) ResourceTypeName() string {
	return "Applications.Link/rabbitMQMessageQueues"
}
