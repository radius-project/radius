// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
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
func (r *RabbitMQMessageQueue) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (r *RabbitMQMessageQueue) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *RabbitMQMessageQueue) ResourceMetadata() *rpv1.BasicResourceProperties {
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
	return linkrp.RabbitMQMessageQueuesResourceType
}

// RabbitMQMessageQueueProperties represents the properties of RabbitMQMessageQueue response resource.
type RabbitMQMessageQueueProperties struct {
	rpv1.BasicResourceProperties
	Queue   string            `json:"queue"`
	Recipe  linkrp.LinkRecipe `json:"recipe,omitempty"`
	Secrets RabbitMQSecrets   `json:"secrets,omitempty"`
	Mode    LinkMode          `json:"mode,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RabbitMQSecrets struct {
	ConnectionString string `json:"connectionString"`
}

func (rabbitmq RabbitMQSecrets) ResourceTypeName() string {
	return linkrp.RabbitMQMessageQueuesResourceType
}
