// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

// ResourceKinds supported. The RP determines how these are created/deleted and the HealthService determines how
// health checks are handled for these
const (
	KindKubernetes = "kubernetes"

	KindAzureCosmosAccountMongo          = "azure.cosmosdb.account.mongo"
	KindAzureCosmosDBMongo               = "azure.cosmosdb.mongo"
	KindAzureCosmosDBSQL                 = "azure.cosmosdb.sql"
	KindAzureKeyVault                    = "azure.keyvault"
	KindAzureKeyVaultSecret              = "azure.keyvault.secret"
	KindAzurePodIdentity                 = "azure.aadpodidentity"
	KindAzureRedis                       = "azure.redis"
	KindAzureRoleAssignment              = "azure.roleassignment"
	KindAzureServiceBusQueue             = "azure.servicebus.queue"
	KindAzureUserAssignedManagedIdentity = "azure.userassignedmanagedidentity"
	KindDaprPubSubTopicAzureServiceBus   = "dapr.pubsubtopic.azureservicebus"
	KindDaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	KindDaprStateStoreSQLServer          = "dapr.statestore.sqlserver"
)
