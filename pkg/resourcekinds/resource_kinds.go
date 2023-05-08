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

package resourcekinds

// ResourceKinds supported. The RP determines how these are created/deleted and the HealthService determines how
// health checks are handled for these
const (
	Azure                            = "azure"
	Kubernetes                       = "kubernetes"
	Deployment                       = "Deployment"
	Service                          = "Service"
	ServiceAccount                   = "ServiceAccount"
	Secret                           = "Secret"
	Gateway                          = "Gateway"
	Volume                           = "Volume"
	KubernetesRole                   = "KubernetesRole"
	KubernetesRoleBinding            = "KubernetesRoleBinding"
	KubernetesHTTPRoute              = "HTTPRoute" // For httproutes.networking.x-k8s.io
	RadiusHttpRoute                  = "HttpRoute" // For httproutes.radius.dev
	StatefulSet                      = "StatefulSet"
	SecretProviderClass              = "SecretProviderClass"
	DaprStateStoreAzureStorage       = "dapr.statestore.azurestorage"
	DaprStateStoreAzureTableService  = "dapr.statestore.azuretableservice"
	DaprStateStoreAzureTable         = "dapr.statestore.azuretable"
	DaprStateStoreAzureTableStorage  = "state.azure.tablestorage"
	DaprGeneric                      = "generic"
	DaprComponent                    = "dapr.io.component"
	DaprPubSubTopicAzureServiceBus   = "pubsub.azure.servicebus"
	AzureCosmosAccount               = "azure.cosmosdb.account"
	AzureCosmosDBMongo               = "azure.cosmosdb.mongo"
	AzureCosmosDBSQL                 = "azure.cosmosdb.sql"
	AzureSqlServer                   = "azure.sql"
	AzureSqlServerDatabase           = "azure.sql.database"
	AzureUserAssignedManagedIdentity = "azure.userassignedmanagedidentity"
	AzureFederatedIdentity           = "azure.federatedidentity"
	AzureRoleAssignment              = "azure.roleassignment"
	AzureRedis                       = "azure.redis"
	AzureFileShare                   = "azure.fileshare"
	AzureFileShareStorageAccount     = "azure.fileshare.account"

	// AnyResourceType is used to designate a resource handler that can handle any type that belongs to a provider. AnyResourceType
	// should only be used to register handlers, and not as part of an output resource.
	AnyResourceType = "*"
)
