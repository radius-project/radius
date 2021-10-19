// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/azurefilesharev1alpha3"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/dapr"
	"github.com/Azure/radius/pkg/renderers/daprhttproutev1alpha3"
	"github.com/Azure/radius/pkg/renderers/daprpubsubv1alpha1"
	"github.com/Azure/radius/pkg/renderers/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/renderers/gateway"
	"github.com/Azure/radius/pkg/renderers/httproutev1alpha3"
	"github.com/Azure/radius/pkg/renderers/keyvaultv1alpha3"
	"github.com/Azure/radius/pkg/renderers/manualscalev1alpha3"
	"github.com/Azure/radius/pkg/renderers/microsoftsqlv1alpha3"
	"github.com/Azure/radius/pkg/renderers/mongodbv1alpha3"

	"github.com/Azure/radius/pkg/renderers/redisv1alpha3"
	"github.com/Azure/radius/pkg/renderers/servicebusqueuev1alpha1"
	"github.com/Azure/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAzureModel(arm armauth.ArmConfig, k8s client.Client) model.ApplicationModel {
	// Configuration for how connections of different types map to role assignments.
	//
	// For a primer on how to read this data, see the KeyVault case.
	roleAssignmentMap := map[string]containerv1alpha3.RoleAssignmentData{

		// Example of how to read this data:
		//
		// For a KeyVault connection...
		// - Look up the dependency based on the connection.Source (azure.com.KeyVaultComponent)
		// - Find the output resource matching LocalID of that dependency (Microsoft.KeyVault/vaults)
		// - Apply the roles in RoleNames (Key Vault Secrets User, Key Vault Crypto User)
		"azure.com/KeyVault": {
			LocalID: outputresource.LocalIDKeyVault,
			RoleNames: []string{
				"Key Vault Secrets User",
				"Key Vault Crypto User",
			},
		},
	}

	rendererMap := map[string]renderers.Renderer{
		// Built-in types
		containerv1alpha3.ResourceType: &dapr.Renderer{
			Inner: &manualscalev1alpha3.Renderer{
				Inner: &containerv1alpha3.Renderer{
					RoleAssignmentMap: roleAssignmentMap,
				},
			},
		},
		httproutev1alpha3.ResourceType: &httproutev1alpha3.Renderer{},

		// Dapr
		daprhttproutev1alpha3.ResourceType: &daprhttproutev1alpha3.Renderer{},
		daprpubsubv1alpha1.ResourceType:    &renderers.V1RendererAdapter{Inner: &daprpubsubv1alpha1.Renderer{}},
		daprstatestorev1alpha1.ResourceType: &renderers.V1RendererAdapter{
			Inner: &daprstatestorev1alpha1.Renderer{
				StateStores: daprstatestorev1alpha1.SupportedAzureStateStoreKindValues,
			},
		},
		gateway.ResourceType: &gateway.Renderer{},

		// Portable
		microsoftsqlv1alpha3.ResourceType: &microsoftsqlv1alpha3.Renderer{},
		mongodbv1alpha3.ResourceType:      &mongodbv1alpha3.AzureRenderer{},
		redisv1alpha3.ResourceType:        &redisv1alpha3.AzureRenderer{},

		// Azure
		keyvaultv1alpha3.ResourceType:        &keyvaultv1alpha3.Renderer{},
		servicebusqueuev1alpha1.ResourceType: &renderers.V1RendererAdapter{Inner: &servicebusqueuev1alpha1.Renderer{}},
		azurefilesharev1alpha3.ResourceType:  &azurefilesharev1alpha3.Renderer{},
	}

	handlerMap := map[string]model.Handlers{
		resourcekinds.Kubernetes:                       {ResourceHandler: handlers.NewKubernetesHandler(k8s), HealthHandler: handlers.NewKubernetesHealthHandler(k8s)},
		resourcekinds.DaprStateStoreAzureStorage:       {ResourceHandler: handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s), HealthHandler: handlers.NewDaprStateStoreAzureStorageHealthHandler(arm, k8s)},
		resourcekinds.DaprStateStoreSQLServer:          {ResourceHandler: handlers.NewDaprStateStoreSQLServerHandler(arm, k8s), HealthHandler: handlers.NewDaprStateStoreSQLServerHealthHandler(arm, k8s)},
		resourcekinds.DaprPubSubTopicAzureServiceBus:   {ResourceHandler: handlers.NewDaprPubSubServiceBusHandler(arm, k8s), HealthHandler: handlers.NewDaprPubSubServiceBusHealthHandler(arm, k8s)},
		resourcekinds.AzureCosmosDBMongo:               {ResourceHandler: handlers.NewAzureCosmosDBMongoHandler(arm), HealthHandler: handlers.NewAzureCosmosDBMongoHealthHandler(arm)},
		resourcekinds.AzureCosmosAccount:               {ResourceHandler: handlers.NewAzureCosmosAccountHandler(arm), HealthHandler: handlers.NewAzureCosmosAccountMongoHealthHandler(arm)},
		resourcekinds.AzureCosmosDBSQL:                 {ResourceHandler: handlers.NewAzureCosmosDBSQLHandler(arm), HealthHandler: handlers.NewAzureCosmosDBSQLHealthHandler(arm)},
		resourcekinds.AzureServiceBusQueue:             {ResourceHandler: handlers.NewAzureServiceBusQueueHandler(arm), HealthHandler: handlers.NewAzureServiceBusQueueHealthHandler(arm)},
		resourcekinds.AzureKeyVault:                    {ResourceHandler: handlers.NewAzureKeyVaultHandler(arm), HealthHandler: handlers.NewAzureKeyVaultHealthHandler(arm)},
		resourcekinds.AzurePodIdentity:                 {ResourceHandler: handlers.NewAzurePodIdentityHandler(arm), HealthHandler: handlers.NewAzurePodIdentityHealthHandler(arm)},
		resourcekinds.AzureSqlServer:                   {ResourceHandler: handlers.NewARMHandler(arm), HealthHandler: handlers.NewARMHealthHandler(arm)},
		resourcekinds.AzureSqlServerDatabase:           {ResourceHandler: handlers.NewARMHandler(arm), HealthHandler: handlers.NewARMHealthHandler(arm)},
		resourcekinds.AzureUserAssignedManagedIdentity: {ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm), HealthHandler: handlers.NewAzureUserAssignedManagedIdentityHealthHandler(arm)},
		resourcekinds.AzureRoleAssignment:              {ResourceHandler: handlers.NewAzureRoleAssignmentHandler(arm), HealthHandler: handlers.NewAzureRoleAssignmentHealthHandler(arm)},
		resourcekinds.AzureKeyVaultSecret:              {ResourceHandler: handlers.NewAzureKeyVaultSecretHandler(arm), HealthHandler: handlers.NewAzureKeyVaultSecretHealthHandler(arm)},
		resourcekinds.AzureRedis:                       {ResourceHandler: handlers.NewAzureRedisHandler(arm), HealthHandler: handlers.NewAzureRedisHealthHandler(arm)},
		resourcekinds.AzureFileShare:                   {ResourceHandler: handlers.NewAzureFileShareHandler(arm), HealthHandler: handlers.NewAzureFileShareHealthHandler(arm)},
		resourcekinds.AzureFileShareStorageAccount:     {ResourceHandler: handlers.NewAzureFileShareStorageAccountHandler(arm), HealthHandler: handlers.NewAzureFileShareStorageAccountHealthHandler(arm)},
	}

	// Lookup of transforms to apply to secrets. By-convention the resource type is used as the
	// key where possible.
	transformerMap := map[string]renderers.SecretValueTransformer{
		mongodbv1alpha3.CosmosMongoResourceType.Type(): &mongodbv1alpha3.AzureTransformer{},
	}

	return model.NewModel(rendererMap, handlerMap, transformerMap)
}
