// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

// ResourceKinds supported.
// TODO: Duplicated from RP for now. Needs to be refactored to share this with RP without adding a dependency on RP
const (
	ResourceKindKubernetes                       = "kubernetes"
	ResourceKindDaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	ResourceKindDaprStateStoreSQLServer          = "dapr.statestore.sqlserver"
	ResourceKindDaprPubSubTopicAzureServiceBus   = "dapr.pubsubtopic.azureservicebus"
	ResourceKindAzureCosmosAccountMongo          = "azure.cosmosdb.account.mongo"
	ResourceKindAzureCosmosDBMongo               = "azure.cosmosdb.mongo"
	ResourceKindAzureCosmosDBSQL                 = "azure.cosmosdb.sql"
	ResourceKindAzureServiceBusQueue             = "azure.servicebus.queue"
	ResourceKindAzureKeyVault                    = "azure.keyvault"
	ResourceKindAzureKeyVaultSecret              = "azure.keyvault.secret"
	ResourceKindAzurePodIdentity                 = "azure.aadpodidentity"
	ResourceKindAzureUserAssignedManagedIdentity = "azure.userassignedmanagedidentity"
	ResourceKindAzureRoleAssignment              = "azure.roleassignment"
	ResourceKindAzureRedis                       = "azure.redis"
)
