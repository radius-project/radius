// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package healthcontract

// ResourceKinds supported. The RP determines how these are created/deleted and the HealthService determines how
// health checks are handled for these
const (
	ResourceKindKubernetes                       = "kubernetes"
	ResourceKindDaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	ResourceKindDaprStateStoreSQLServer          = "dapr.statestore.sqlserver"
	ResourceKindDaprPubSubTopicAzureServiceBus   = "dapr.pubsubtopic.azureservicebus"
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
