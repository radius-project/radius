// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

// Resource kind
const (
	KindKubernetes                       = "kubernetes"
	KindDaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	KindDaprStateStoreSQLServer          = "dapr.statestore.sqlserver"
	KindDaprPubSubTopicAzureServiceBus   = "dapr.pubsubtopic.azureservicebus"
	KindAzureCosmosDBMongo               = "azure.cosmosdb.mongo"
	KindAzureCosmosDBSQL                 = "azure.cosmosdb.sql"
	KindAzureRedis                       = "azure.redis"
	KindAzureServiceBusQueue             = "azure.servicebus.queue"
	KindAzureKeyVault                    = "azure.keyvault"
	KindAzureKeyVaultSecret              = "azure.keyvault.secret"
	KindAzurePodIdentity                 = "azure.aadpodidentity"
	KindAzureUserAssignedManagedIdentity = "azure.userassignedmanagedidentity"
	KindAzureRoleAssignment              = "azure.roleassignment"
)
