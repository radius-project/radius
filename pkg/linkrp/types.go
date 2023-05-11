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

package linkrp

type RecipeData struct {
	RecipeProperties

	// APIVersion is the API version to use to perform operations on resources supported by the link.
	// For example for Azure resources, every service has different REST API version that must be specified in the request.
	APIVersion string

	// Resource ids of the resources deployed by the recipe
	Resources []string
}

// RecipeProperties represents the information needed to deploy a recipe
type RecipeProperties struct {
	LinkRecipe                   // LinkRecipe is the recipe of the resource to be deployed
	LinkType      string         // LinkType represent the type of the link
	TemplatePath  string         // TemplatePath represent the recipe location
	EnvParameters map[string]any // EnvParameters represents the parameters set by the operator while linking the recipe to an environment
}

// LinkRecipe is the recipe details used to automatically deploy underlying infrastructure for a link
type LinkRecipe struct {
	// Name of the recipe within the environment to use
	Name string `json:"name,omitempty"`
	// Parameters are key/value parameters to pass into the recipe at deployment
	Parameters map[string]any `json:"parameters,omitempty"`
}

// ResourceReference represents a reference to a resource that was deployed by the user
// and specified as part of a link resource.
//
// This type should be used in datamodels for the '.properties.resources' field.
type ResourceReference struct {
	ID string `json:"id"`
}

// RecipeContext Recipe template authors can leverage the RecipeContext parameter to access Link properties to
// generate name and properties that are unique for the Link calling the recipe.
type RecipeContext struct {
	Resource    Resource     `json:"resource,omitempty"`
	Application ResourceInfo `json:"application,omitempty"`
	Environment ResourceInfo `json:"environment,omitempty"`
	Runtime     Runtime      `json:"runtime,omitempty"`
}

// Resource contains the information needed to deploy a recipe.
// In the case the resource is a Link, it represents the Link's id, name and type.
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

type Kubernetes struct {
	// Namespace is set to the applicationNamespace when the Link is application-scoped, and set to the environmentNamespace when the Link is environment scoped
	Namespace string `json:"namespace"`
	// EnvironmentNamespace is set to environment namespace.
	EnvironmentNamespace string `json:"environmentNamespace"`
}

const (
	DaprInvokeHttpRoutesResourceType  = "Applications.Link/daprInvokeHttpRoutes"
	DaprPubSubBrokersResourceType     = "Applications.Link/daprPubSubBrokers"
	DaprSecretStoresResourceType      = "Applications.Link/daprSecretStores"
	DaprStateStoresResourceType       = "Applications.Link/daprStateStores"
	ExtendersResourceType             = "Applications.Link/extenders"
	MongoDatabasesResourceType        = "Applications.Link/mongoDatabases"
	RabbitMQMessageQueuesResourceType = "Applications.Link/rabbitMQMessageQueues"
	RedisCachesResourceType           = "Applications.Link/redisCaches"
	SqlDatabasesResourceType          = "Applications.Link/sqlDatabases"
)
