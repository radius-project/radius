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
	Secret                           = "Secret"
	Gateway                          = "Gateway"
	KubernetesHTTPRoute              = "HTTPRoute" // For httproutes.networking.x-k8s.io
	RadiusHttpRoute                  = "HttpRoute" // For httproutes.radius.dev
	StatefulSet                      = "StatefulSet"
	SecretProviderClass              = "SecretProviderClass"
	DaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	DaprPubSubTopicGeneric           = "generic"
	DaprPubSubTopicAzureServiceBus   = "pubsub.azure.servicebus"
	DaprStateStoreGeneric            = "dapr.io.statestore"
	DaprSecretStoreGeneric           = "dapr.io.secretstore"
	DaprPubSubGeneric                = "dapr.io.pubsubtopic"
	AzureCosmosAccount               = "azure.cosmosdb.account"
	AzureCosmosDBMongo               = "azure.cosmosdb.mongo"
	AzureCosmosDBSQL                 = "azure.cosmosdb.sql"
	AzureServiceBusQueue             = "Microsoft.ServiceBus"
	AzureSqlServer                   = "azure.sql"
	AzureSqlServerDatabase           = "azure.sql.database"
	AzureKeyVault                    = "azure.keyvault"
	AzureKeyVaultSecret              = "azure.keyvault.secret"
	AzurePodIdentity                 = "azure.aadpodidentity"
	AzureUserAssignedManagedIdentity = "azure.userassignedmanagedidentity"
	AzureRoleAssignment              = "azure.roleassignment"
	AzureRedis                       = "azure.redis"
	AzureFileShare                   = "azure.fileshare"
	AzureFileShareStorageAccount     = "azure.fileshare.account"
)
