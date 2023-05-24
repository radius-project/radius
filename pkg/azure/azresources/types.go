/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Contains constant values related to azure resources
package azresources

const (
	AzureFileShareFileServices                 = "fileServices"
	AzureFileShareShares                       = "shares"
	ContainerServiceManagedClusters            = "Microsoft.ContainerService/managedClusters"
	DocumentDBDatabaseAccounts                 = "Microsoft.DocumentDB/databaseAccounts"
	CacheRedis                                 = "Microsoft.Cache/redis"
	DocumentDBDatabaseAccountsMongoDBDatabases = "mongodbDatabases"
	DocumentDBDatabaseAccountsSQLDatabases     = "sqlDatabases"
	KeyVaultVaults                             = "Microsoft.KeyVault/vaults"
	KeyVaultVaultsSecrets                      = "secrets"
	ManagedIdentityUserAssignedIdentities      = "Microsoft.ManagedIdentity/userAssignedIdentities"
	ServiceBusNamespaces                       = "Microsoft.ServiceBus/namespaces"
	ServiceBusNamespacesQueues                 = "queues"
	ServiceBusNamespacesTopics                 = "topics"
	StorageStorageAccounts                     = "Microsoft.Storage/storageAccounts"
	StorageStorageAccountsTables               = "tables"
	StorageStorageTableServices                = "tableServices"
	SqlServers                                 = "Microsoft.Sql/servers"
	SqlServersDatabases                        = "databases"
)
