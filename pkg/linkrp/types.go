// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package linkrp

type RecipeData struct {
	RecipeProperties

	Provider string

	// API version to use to perform operations on resources supported by the link.
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
