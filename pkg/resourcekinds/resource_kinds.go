// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcekinds

// ResourceKinds supported. The RP determines how these are created/deleted and the HealthService determines how
// health checks are handled for these
const (
	Azure                            = "azure"
	Kubernetes                       = "kubernetes"
	Deployment                       = "Deployment"
	Service                          = "Service"
	ServiceAccount                   = "ServiceAccount"
	Secret                           = "Secret"
	Gateway                          = "Gateway"
	Volume                           = "Volume"
	KubernetesRole                   = "KubernetesRole"
	KubernetesRoleBinding            = "KubernetesRoleBinding"
	KubernetesHTTPRoute              = "HTTPRoute" // For httproutes.networking.x-k8s.io
	RadiusHttpRoute                  = "HttpRoute" // For httproutes.radius.dev
	StatefulSet                      = "StatefulSet"
	SecretProviderClass              = "SecretProviderClass"
	DaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	DaprStateStoreAzureTableStorage  = "state.azure.tablestorage"
	DaprGeneric                      = "generic"
	DaprComponent                    = "dapr.io.component"
	DaprPubSubTopicAzureServiceBus   = "pubsub.azure.servicebus"
	AzureCosmosAccount               = "azure.cosmosdb.account"
	AzureCosmosDBMongo               = "azure.cosmosdb.mongo"
	AzureCosmosDBSQL                 = "azure.cosmosdb.sql"
	AzureSqlServer                   = "azure.sql"
	AzureSqlServerDatabase           = "azure.sql.database"
	AzureUserAssignedManagedIdentity = "azure.userassignedmanagedidentity"
	AzureFederatedIdentity           = "azure.federatedidentity"
	AzureRoleAssignment              = "azure.roleassignment"
	AzureRedis                       = "azure.redis"
	AzureFileShare                   = "azure.fileshare"
	AzureFileShareStorageAccount     = "azure.fileshare.account"

	// Wildcard is used to designate a resource handler that can handle any type that belongs to a provider. Wildcard
	// should only be used to register handlers, and not as part of an output resource.
	Wildcard = "*"
)
