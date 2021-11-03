// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/dapr"
	"github.com/Azure/radius/pkg/renderers/daprhttproutev1alpha3"
	"github.com/Azure/radius/pkg/renderers/daprpubsubv1alpha3"
	"github.com/Azure/radius/pkg/renderers/daprstatestorev1alpha3"
	"github.com/Azure/radius/pkg/renderers/gateway"
	"github.com/Azure/radius/pkg/renderers/httproutev1alpha3"
	"github.com/Azure/radius/pkg/renderers/keyvaultv1alpha3"
	"github.com/Azure/radius/pkg/renderers/manualscalev1alpha3"
	"github.com/Azure/radius/pkg/renderers/microsoftsqlv1alpha3"
	"github.com/Azure/radius/pkg/renderers/mongodbv1alpha3"
	"github.com/Azure/radius/pkg/renderers/volumev1alpha3"
	"github.com/Azure/radius/pkg/resourcemodel"

	"github.com/Azure/radius/pkg/renderers/redisv1alpha3"
	"github.com/Azure/radius/pkg/renderers/servicebusqueuev1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAzureModel(arm armauth.ArmConfig, k8s client.Client) model.ApplicationModel {
	// Configuration for how connections of different types map to role assignments.
	//
	// For a primer on how to read this data, see the KeyVault case.
	roleAssignmentMap := map[radclient.ContainerConnectionKind]containerv1alpha3.RoleAssignmentData{

		// Example of how to read this data:
		//
		// For a KeyVault connection...
		// - Look up the dependency based on the connection.Source (azure.com.KeyVaultComponent)
		// - Find the output resource matching LocalID of that dependency (Microsoft.KeyVault/vaults)
		// - Apply the roles in RoleNames (Key Vault Secrets User, Key Vault Crypto User)
		radclient.ContainerConnectionKindAzureComKeyVault: {
			LocalID: outputresource.LocalIDKeyVault,
			RoleNames: []string{
				"Key Vault Secrets User",
				"Key Vault Crypto User",
			},
		},
	}

	radiusResources := []model.RadiusResourceModel{
		// Built-in types
		{
			ResourceType: containerv1alpha3.ResourceType,
			Renderer: &dapr.Renderer{
				Inner: &manualscalev1alpha3.Renderer{
					Inner: &containerv1alpha3.Renderer{
						RoleAssignmentMap: roleAssignmentMap,
					},
				},
			},
		},
		{
			ResourceType: httproutev1alpha3.ResourceType,
			Renderer:     &httproutev1alpha3.Renderer{},
		},

		// Dapr
		{
			ResourceType: daprhttproutev1alpha3.ResourceType,
			Renderer:     &daprhttproutev1alpha3.Renderer{},
		},
		{
			ResourceType: daprpubsubv1alpha3.ResourceType,
			Renderer:     &daprpubsubv1alpha3.Renderer{},
		},
		{
			ResourceType: daprstatestorev1alpha3.ResourceType,
			Renderer: &daprstatestorev1alpha3.Renderer{
				StateStores: daprstatestorev1alpha3.SupportedAzureStateStoreKindValues,
			},
		},

		// Portable
		{
			ResourceType: microsoftsqlv1alpha3.ResourceType,
			Renderer:     &microsoftsqlv1alpha3.Renderer{},
		},
		{
			ResourceType: mongodbv1alpha3.ResourceType,
			Renderer:     &mongodbv1alpha3.AzureRenderer{},
		},
		{
			ResourceType: redisv1alpha3.ResourceType,
			Renderer:     &redisv1alpha3.AzureRenderer{},
		},
		{
			ResourceType: gateway.ResourceType,
			Renderer:     &gateway.Renderer{},
		},

		// Azure
		{
			ResourceType: keyvaultv1alpha3.ResourceType,
			Renderer:     &keyvaultv1alpha3.Renderer{},
		},
		{
			ResourceType: volumev1alpha3.ResourceType,
			Renderer:     &volumev1alpha3.AzureRenderer{VolumeRenderers: volumev1alpha3.SupportedVolumeRenderers},
		},
		{
			ResourceType: servicebusqueuev1alpha3.ResourceType,
			Renderer:     &servicebusqueuev1alpha3.Renderer{},
		},
	}

	skipHealthCheckKubernetesKinds := map[string]bool{
		resourcekinds.Service:     true,
		resourcekinds.Secret:      true,
		resourcekinds.StatefulSet: true,
		resourcekinds.HTTPRoute:   true,
	}

	outputResources := []model.OutputResourceModel{
		{
			Kind:            resourcekinds.Kubernetes,
			HealthHandler:   handlers.NewKubernetesHealthHandler(k8s),
			ResourceHandler: handlers.NewKubernetesHandler(k8s),

			// We can monitor specific kinds of Kubernetes resources for health tracking, but not all of them.
			ShouldSupportHealthMonitorFunc: func(identity resourcemodel.ResourceIdentity) bool {
				if identity.Kind == resourcemodel.IdentityKindKubernetes {
					skip := skipHealthCheckKubernetesKinds[identity.Data.(resourcemodel.KubernetesIdentity).Kind]
					return !skip
				}

				return false
			},
		},
		{
			Kind:            resourcekinds.DaprStateStoreAzureStorage,
			HealthHandler:   handlers.NewDaprStateStoreAzureStorageHealthHandler(arm, k8s),
			ResourceHandler: handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s),
		},
		{
			Kind:            resourcekinds.DaprStateStoreSQLServer,
			HealthHandler:   handlers.NewDaprStateStoreSQLServerHealthHandler(arm, k8s),
			ResourceHandler: handlers.NewDaprStateStoreSQLServerHandler(arm, k8s),
		},
		{
			Kind:            resourcekinds.DaprPubSubTopicAzureServiceBus,
			HealthHandler:   handlers.NewDaprPubSubServiceBusHealthHandler(arm, k8s),
			ResourceHandler: handlers.NewDaprPubSubServiceBusHandler(arm, k8s),
		},
		{
			Kind:                   resourcekinds.AzureCosmosDBMongo,
			HealthHandler:          handlers.NewAzureCosmosDBMongoHealthHandler(arm),
			ResourceHandler:        handlers.NewAzureCosmosDBMongoHandler(arm),
			SecretValueTransformer: &mongodbv1alpha3.AzureTransformer{},
		},
		{
			Kind:            resourcekinds.AzureCosmosAccount,
			HealthHandler:   handlers.NewAzureCosmosAccountMongoHealthHandler(arm),
			ResourceHandler: handlers.NewAzureCosmosAccountHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureCosmosDBSQL,
			HealthHandler:   handlers.NewAzureCosmosDBSQLHealthHandler(arm),
			ResourceHandler: handlers.NewAzureCosmosDBSQLHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureServiceBusQueue,
			HealthHandler:   handlers.NewAzureServiceBusQueueHealthHandler(arm),
			ResourceHandler: handlers.NewAzureServiceBusQueueHandler(arm),
			ShouldSupportHealthMonitorFunc: func(identity resourcemodel.ResourceIdentity) bool {
				return true
			},
		},
		{
			Kind:            resourcekinds.AzureKeyVault,
			HealthHandler:   handlers.NewAzureKeyVaultHealthHandler(arm),
			ResourceHandler: handlers.NewAzureKeyVaultHandler(arm),
		},
		{
			Kind:            resourcekinds.AzurePodIdentity,
			HealthHandler:   handlers.NewAzurePodIdentityHealthHandler(arm),
			ResourceHandler: handlers.NewAzurePodIdentityHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureSqlServer,
			HealthHandler:   handlers.NewARMHealthHandler(arm),
			ResourceHandler: handlers.NewARMHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureSqlServerDatabase,
			HealthHandler:   handlers.NewARMHealthHandler(arm),
			ResourceHandler: handlers.NewARMHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureUserAssignedManagedIdentity,
			HealthHandler:   handlers.NewAzureUserAssignedManagedIdentityHealthHandler(arm),
			ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureRoleAssignment,
			HealthHandler:   handlers.NewAzureRoleAssignmentHealthHandler(arm),
			ResourceHandler: handlers.NewAzureRoleAssignmentHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureKeyVaultSecret,
			HealthHandler:   handlers.NewAzureKeyVaultSecretHealthHandler(arm),
			ResourceHandler: handlers.NewAzureKeyVaultSecretHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureRedis,
			HealthHandler:   handlers.NewAzureRedisHealthHandler(arm),
			ResourceHandler: handlers.NewAzureRedisHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureFileShare,
			HealthHandler:   handlers.NewAzureFileShareHealthHandler(arm),
			ResourceHandler: handlers.NewAzureFileShareHandler(arm),
		},
		{
			Kind:            resourcekinds.AzureFileShareStorageAccount,
			HealthHandler:   handlers.NewAzureFileShareStorageAccountHealthHandler(arm),
			ResourceHandler: handlers.NewAzureFileShareStorageAccountHandler(arm),
		},
	}

	return model.NewModel(radiusResources, outputResources)
}
