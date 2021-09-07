// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcekinds

// Azure ResourceKinds supported.
const (
	KindKubernetes                       = "kubernetes"
	KindDeployment                       = "Deployment"
	KindService                          = "Service"
	KindDaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	KindDaprStateStoreSQLServer          = "dapr.statestore.sqlserver"
	KindDaprPubSubTopicAzureServiceBus   = "dapr.pubsubtopic.azureservicebus"
	KindAzureCosmosAccountMongo          = "azure.cosmosdb.account.mongo"
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
