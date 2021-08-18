// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

// ResourceKinds supported. The RP determines how these are created/deleted and the HealthService determines how
// health checks are handled for these
const (
	KindKubernetes                       = "kubernetes"
	KindDaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	KindDaprStateStoreSQLServer          = "dapr.statestore.sqlserver"
	KindDaprPubSubTopicAzureServiceBus   = "dapr.pubsubtopic.azureservicebus"
	KindAzureCosmosDBMongo               = "azure.cosmosdb.mongo"
	KindAzureCosmosDBSQL                 = "azure.cosmosdb.sql"
	KindAzureServiceBusQueue             = "azure.servicebus.queue"
	KindAzureKeyVault                    = "azure.keyvault"
	KindAzureKeyVaultSecret              = "azure.keyvault.secret"
	KindAzurePodIdentity                 = "azure.aadpodidentity"
	KindAzureUserAssignedManagedIdentity = "azure.userassignedmanagedidentity"
	KindAzureRoleAssignment              = "azure.roleassignment"
	KindAzureRedis                       = "azure.redis"
)
