// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Contains constant values related to azure resources
package azresources

import "strings"

const (
	AzureFileShareFileServices                  = "fileServices"
	AzureFileShareShares                        = "shares"
	ContainerServiceManagedClusters             = "Microsoft.ContainerService/managedClusters"
	CustomProvidersResourceProviders            = "Microsoft.CustomProviders/resourceProviders"
	DocumentDBDatabaseAccounts                  = "Microsoft.DocumentDB/databaseAccounts"
	CacheRedis                                  = "Microsoft.Cache/redis"
	DocumentDBDatabaseAccountsMongodDBDatabases = "mongodbDatabases"
	DocumentDBDatabaseAccountsSQLDatabases      = "sqlDatabases"
	KeyVaultVaults                              = "Microsoft.KeyVault/vaults"
	KeyVaultVaultsSecrets                       = "secrets"
	ManagedIdentityUserAssignedIdentities       = "Microsoft.ManagedIdentity/userAssignedIdentities"
	ResourcesDeploymentScripts                  = "Microsoft.Resources/deploymentScripts"
	ServiceBusNamespaces                        = "Microsoft.ServiceBus/namespaces"
	ServiceBusNamespacesQueues                  = "queues"
	ServiceBusNamespacesTopics                  = "topics"
	StorageStorageAccounts                      = "Microsoft.Storage/storageAccounts"
	StorageStorageAccountsTables                = "tables"
	SqlServers                                  = "Microsoft.Sql/servers"
	SqlServersDatabases                         = "databases"
	WebServerFarms                              = "Microsoft.Web/serverFarms"
	WebSites                                    = "Microsoft.Web/sites"

	CustomRPV3Name     = "radiusv3"
	CustomRPApiVersion = "2018-09-01-preview"
)

func IsRadiusCustomAction(id ResourceID) bool {
	if len(id.Types) == 1 &&
		strings.EqualFold(id.Types[0].Type, CustomProvidersResourceProviders) &&
		strings.EqualFold(id.Types[0].Name, CustomRPV3Name) {
		return true
	}

	return false
}

func IsRadiusResource(id ResourceID) bool {
	if len(id.Types) >= 2 &&
		strings.EqualFold(id.Types[0].Type, CustomProvidersResourceProviders) &&
		strings.EqualFold(id.Types[0].Name, CustomRPV3Name) {
		return true
	}

	return false
}

func IsKubernetesResource(id ResourceID) bool {
	if len(id.Types) >= 1 &&
		strings.HasPrefix(strings.ToLower(id.Types[0].Type), "kubernetes.") {
		return true
	}

	return false
}
