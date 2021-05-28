// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloads

// Resource Kinds
const (
	ResourceKindKubernetes                       = "kubernetes"
	ResourceKindDaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	ResourceKindDaprStateStoreSQLServer          = "dapr.statestore.sqlserver"
	ResourceKindDaprPubSubTopicAzureServiceBus   = "dapr.pubsubtopic.azureservicebus"
	ResourceKindAzureCosmosDBMongo               = "azure.cosmosdb.mongo"
	ResourceKindAzureCosmosDBSQL                 = "azure.cosmosdb.sql"
	ResourceKindAzureServiceBusQueue             = "azure.servicebus.queue"
	ResourceKindAzureKeyVault                    = "azure.keyvault"
	ResourceKindAzurePodIdentity                 = "azure.aadpodidentity"
	ResourceKindAzureUserAssignedManagedIdentity = "azure.userassignedmanagedidentity"
)
