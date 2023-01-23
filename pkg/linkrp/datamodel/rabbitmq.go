// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

// RabbitMQMessageQueue represents RabbitMQMessageQueue link resource.
type RabbitMQMessageQueue struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RabbitMQMessageQueueProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (r *RabbitMQMessageQueue) Transform(outputResources []outputresource.OutputResource, computedValues map[string]any, secretValues map[string]rp.SecretValueReference) error {
	r.Properties.Status.OutputResources = outputResources
	r.ComputedValues = computedValues
	r.SecretValues = secretValues
	if queue, ok := computedValues[linkrp.QueueNameKey].(string); ok {
		r.Properties.Queue = queue
	}

	return nil
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *RabbitMQMessageQueue) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
}

// OutputResources returns the output resources array.
func (r *RabbitMQMessageQueue) OutputResources() []outputresource.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *RabbitMQMessageQueue) ResourceMetadata() *rp.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ComputedValues returns the computed values on the link.
func (r *RabbitMQMessageQueue) GetComputedValues() map[string]any {
	return r.LinkMetadata.ComputedValues
}

// SecretValues returns the secret values for the link.
func (r *RabbitMQMessageQueue) GetSecretValues() map[string]rp.SecretValueReference {
	return r.LinkMetadata.SecretValues
}

// RecipeData returns the recipe data for the link.
func (r *RabbitMQMessageQueue) GetRecipeData() RecipeData {
	return r.LinkMetadata.RecipeData
}

func (rabbitmq *RabbitMQMessageQueue) ResourceTypeName() string {
	return "Applications.Link/rabbitMQMessageQueues"
}

// RabbitMQMessageQueueProperties represents the properties of RabbitMQMessageQueue response resource.
type RabbitMQMessageQueueProperties struct {
	rp.BasicResourceProperties
	Queue   string          `json:"queue"`
	Recipe  LinkRecipe      `json:"recipe,omitempty"`
	Secrets RabbitMQSecrets `json:"secrets,omitempty"`
	Mode    LinkMode        `json:"mode,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RabbitMQSecrets struct {
	ConnectionString string `json:"connectionString"`
}

func (rabbitmq RabbitMQSecrets) ResourceTypeName() string {
	return "Applications.Link/rabbitMQMessageQueues"
}
