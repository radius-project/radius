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

package portableresources

import (
	"strings"
)

const (
	// DaprPubSubBrokersResourceType represents the resource type for Dapr PubSub brokers.
	DaprPubSubBrokersResourceType = "Applications.Dapr/pubSubBrokers"

	// DaprSecretStoresResourceType represents the resource type for Dapr Secret stores.
	DaprSecretStoresResourceType = "Applications.Dapr/secretStores"

	// DaprStateStoresResourceType represents the resource type for Dapr State stores.
	DaprStateStoresResourceType = "Applications.Dapr/stateStores"

	// RabbitMQQueuesResourceType represents the resource type for RabbitMQ queue.
	RabbitMQQueuesResourceType = "Applications.Messaging/rabbitMQQueues"

	// MongoDatabasesResourceType represents the resource type for Mongo database.
	MongoDatabasesResourceType = "Applications.Datastores/mongoDatabases"

	// RedisCachesResourceType represents the resource type for Redis caches.
	RedisCachesResourceType = "Applications.Datastores/redisCaches"

	// SqlDatabasesResourceType represents the resource type for SQL databases.
	SqlDatabasesResourceType = "Applications.Datastores/sqlDatabases"

	// ExtendersResourceType represents the resource type for Extenders resource.
	ExtendersResourceType = "Applications.Core/extenders"

	// ResourceProvisioningRecipe is the scenario when Radius manages the lifecycle of the resource through a Recipe.
	ResourceProvisioningRecipe ResourceProvisioning = "recipe"

	// ResourceProvisioningManual is the scenario where the user manages the resource and provides values.
	ResourceProvisioningManual ResourceProvisioning = "manual"

	// DefaultRecipeName represents the default recipe name.
	DefaultRecipeName = "default"
)

type RecipeData struct {
	RecipeProperties

	// APIVersion is the API version to use to perform operations on resources.
	// For example for Azure resources, every service has different REST API version that must be specified in the request.
	APIVersion string

	// Resource ids of the resources deployed by the recipe
	Resources []string
}

// RecipeProperties represents the information needed to deploy a recipe
type RecipeProperties struct {
	ResourceRecipe                // ResourceRecipe is the recipe of the resource to be deployed
	ResourceType   string         // ResourceType represent the type of the resource
	TemplatePath   string         // TemplatePath represent the recipe location
	EnvParameters  map[string]any // EnvParameters represents the parameters set by the operator while linking the recipe to an environment
}

// ResourceRecipe is the recipe details used to automatically deploy underlying infrastructure for a resource.
type ResourceRecipe struct {
	// Name of the recipe within the environment to use
	Name string `json:"name,omitempty"`
	// Parameters are key/value parameters to pass into the recipe at deployment
	Parameters map[string]any `json:"parameters,omitempty"`
	// Recipe Deployment Status
	DeploymentStatus RecipeDeploymentStatus `json:"recipeStatus,omitempty"`
}

// ResourceReference represents a reference to a resource that was deployed by the user
// and specified as part of a portable resource.
//
// This type should be used in datamodels for the '.properties.resources' field.
type ResourceReference struct {
	ID string `json:"id"`
}

// RecipeContext Recipe template authors can leverage the RecipeContext parameter to access portable resource properties to
// generate name and properties that are unique for the resource calling the recipe.
type RecipeContext struct {
	Resource    Resource     `json:"resource,omitempty"`
	Application ResourceInfo `json:"application,omitempty"`
	Environment ResourceInfo `json:"environment,omitempty"`
	Runtime     Runtime      `json:"runtime,omitempty"`
}

// Resource contains the information needed to deploy a recipe.
// In the case the resource is a portable resource, it represents the resource's id, name and type.
type Resource struct {
	ResourceInfo
	Type string `json:"type"`
}

// ResourceInfo name and id of the resource
type ResourceInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type Runtime struct {
	Kubernetes Kubernetes `json:"kubernetes,omitempty"`
}

// ResourceProvisioning specifies how the resource should be managed
type ResourceProvisioning string

type Kubernetes struct {
	// Namespace is set to the applicationNamespace when the portable resource is application-scoped, and set to the environmentNamespace when it is environment scoped
	Namespace string `json:"namespace"`
	// EnvironmentNamespace is set to environment namespace.
	EnvironmentNamespace string `json:"environmentNamespace"`
}

type RecipeDeploymentStatus string

const (
	// ExecutionError represents a failure status during recipe execution.
	ExecutionError RecipeDeploymentStatus = "executionError"
	// RecipeSetupError represents a failure that happens before a recipe or output resources are deployed.
	RecipeSetupError RecipeDeploymentStatus = "setupError"
	// Success represents a successful recipe execution.
	Success RecipeDeploymentStatus = "success"
)

// IsValidPortableResourceType checks if the provided resource type is a valid portable resource type.
// Returns true if the resource type is valid, false otherwise.
func IsValidPortableResourceType(resourceType string) bool {
	portableResourceTypes := []string{
		DaprPubSubBrokersResourceType,
		DaprSecretStoresResourceType,
		DaprStateStoresResourceType,
		RabbitMQQueuesResourceType,
		MongoDatabasesResourceType,
		RedisCachesResourceType,
		SqlDatabasesResourceType,
		ExtendersResourceType,
	}
	for _, s := range portableResourceTypes {
		if strings.EqualFold(s, resourceType) {
			return true
		}
	}

	return false
}
