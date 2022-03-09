// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/model"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/dapr"
	"github.com/project-radius/radius/pkg/renderers/daprhttproutev1alpha3"
	"github.com/project-radius/radius/pkg/renderers/daprpubsubv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/daprstatestorev1alpha3"
	"github.com/project-radius/radius/pkg/renderers/extenderv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/gateway"
	"github.com/project-radius/radius/pkg/renderers/httproutev1alpha3"
	"github.com/project-radius/radius/pkg/renderers/keyvaultv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/manualscalev1alpha3"
	"github.com/project-radius/radius/pkg/renderers/microsoftsqlv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/mongodbv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/rabbitmqv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/volumev1alpha3"
	"github.com/project-radius/radius/pkg/resourcemodel"

	"github.com/project-radius/radius/pkg/renderers/redisv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/servicebusqueuev1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAzureModel(arm armauth.ArmConfig, k8s client.Client) model.ApplicationModel {
	// Configure RBAC support on connections based connection kind.
	// Role names can be user input or default roles assigned by Radius.
	// Leave RoleNames field empty if no default roles are supported for a connection kind.
	//
	// For a primer on how to read this data, see the KeyVault case.
	roleAssignmentMap := map[radclient.ConnectionKind]containerv1alpha3.RoleAssignmentData{

		// Example of how to read this data:
		//
		// For a KeyVault connection...
		// - Look up the dependency based on the connection.Source (azure.com.KeyVault)
		// - Find the output resource matching LocalID of that dependency (Microsoft.KeyVault/vaults)
		// - Apply the roles in RoleNames (Key Vault Secrets User, Key Vault Crypto User)
		radclient.ConnectionKindAzureComKeyVault: {
			LocalID: outputresource.LocalIDKeyVault,
			RoleNames: []string{
				"Key Vault Secrets User",
				"Key Vault Crypto User",
			},
		},
		radclient.ConnectionKindAzure: {
			// RBAC for non-Radius Azure resources. Supports user specified roles.
			// More information can be found here: https://github.com/project-radius/radius/issues/1321
		},
	}

	radiusResourceModel := []model.RadiusResourceModel{
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
			Renderer: &daprpubsubv1alpha3.Renderer{
				PubSubs: daprpubsubv1alpha3.SupportedAzurePubSubKindValues,
			},
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
			ResourceType: rabbitmqv1alpha3.ResourceType,
			Renderer:     &rabbitmqv1alpha3.AzureRenderer{},
		},
		{
			ResourceType: gateway.ResourceType,
			Renderer:     &gateway.Renderer{},
		},
		{
			ResourceType: extenderv1alpha3.ResourceType,
			Renderer:     &extenderv1alpha3.AzureRenderer{},
		},

		// Azure
		{
			ResourceType: keyvaultv1alpha3.ResourceType,
			Renderer:     &keyvaultv1alpha3.Renderer{},
		},
		{
			ResourceType: volumev1alpha3.ResourceType,
			Renderer:     &volumev1alpha3.AzureRenderer{VolumeRenderers: volumev1alpha3.GetSupportedRenderers(), Arm: arm},
		},
		{
			ResourceType: servicebusqueuev1alpha3.ResourceType,
			Renderer:     &servicebusqueuev1alpha3.Renderer{},
		},
	}

	skipHealthCheckKubernetesKinds := map[string]bool{
		resourcekinds.Service:             true,
		resourcekinds.Secret:              true,
		resourcekinds.StatefulSet:         true,
		resourcekinds.KubernetesHTTPRoute: true,
		resourcekinds.SecretProviderClass: true,
	}

	outputResourceModel := []model.OutputResourceModel{
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
			Kind:            resourcekinds.DaprStateStoreGeneric,
			HealthHandler:   handlers.NewDaprStateStoreGenericHealthHandler(arm, k8s),
			ResourceHandler: handlers.NewDaprStateStoreGenericHandler(arm, k8s),
		},
		{
			Kind:            resourcekinds.DaprPubSubTopicAzureServiceBus,
			HealthHandler:   handlers.NewDaprPubSubServiceBusHealthHandler(arm, k8s),
			ResourceHandler: handlers.NewDaprPubSubServiceBusHandler(arm, k8s),
		},
		{
			Kind:            resourcekinds.DaprPubSubTopicGeneric,
			HealthHandler:   handlers.NewDaprPubSubGenericHealthHandler(arm, k8s),
			ResourceHandler: handlers.NewDaprPubSubGenericHandler(arm, k8s),
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

	return model.NewModel(radiusResourceModel, outputResourceModel)
}
