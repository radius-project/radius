// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/model"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/dapr"
	"github.com/project-radius/radius/pkg/renderers/daprhttproutev1alpha3"
	"github.com/project-radius/radius/pkg/renderers/daprpubsubv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/daprsecretstorev1alpha3"
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

func NewAzureModel(arm *armauth.ArmConfig, k8s client.Client) (model.ApplicationModel, error) {
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

	// Configure the providers supported by the appmodel
	supportedProviders := map[string]bool{
		providers.ProviderKubernetes: true,
	}
	if arm != nil {
		supportedProviders[providers.ProviderAzure] = true
		supportedProviders[providers.ProviderAzureKubernetesService] = true
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
		{
			ResourceType: daprsecretstorev1alpha3.ResourceType,
			Renderer: &daprsecretstorev1alpha3.Renderer{
				SecretStores: daprsecretstorev1alpha3.SupportedAzureSecretStoreKindValues,
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
	}

	if arm != nil {
		azureModel := []model.RadiusResourceModel{
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

		radiusResourceModel = append(radiusResourceModel, azureModel...)
	}

	supportedHealthCheckKubernetesKinds := map[string]bool{
		resourcekinds.Deployment: true,
	}

	shouldSupportHealthMonitorFunc := func(resourceType resourcemodel.ResourceType) bool {
		if resourceType.Provider == providers.ProviderKubernetes {
			return supportedHealthCheckKubernetesKinds[resourceType.Type]
		}

		return false
	}

	outputResourceModel := []model.OutputResourceModel{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Kubernetes,
				Provider: providers.ProviderKubernetes,
			},
			HealthHandler:   handlers.NewKubernetesHealthHandler(k8s),
			ResourceHandler: handlers.NewKubernetesHandler(k8s),

			// We can monitor specific kinds of Kubernetes resources for health tracking, but not all of them.
			ShouldSupportHealthMonitorFunc: shouldSupportHealthMonitorFunc,
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: providers.ProviderKubernetes,
			},
			HealthHandler:                  handlers.NewKubernetesHealthHandler(k8s),
			ResourceHandler:                handlers.NewKubernetesHandler(k8s),
			ShouldSupportHealthMonitorFunc: shouldSupportHealthMonitorFunc,
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Service,
				Provider: providers.ProviderKubernetes,
			},
			HealthHandler:                  handlers.NewKubernetesHealthHandler(k8s),
			ResourceHandler:                handlers.NewKubernetesHandler(k8s),
			ShouldSupportHealthMonitorFunc: shouldSupportHealthMonitorFunc,
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Secret,
				Provider: providers.ProviderKubernetes,
			},
			HealthHandler:                  handlers.NewKubernetesHealthHandler(k8s),
			ResourceHandler:                handlers.NewKubernetesHandler(k8s),
			ShouldSupportHealthMonitorFunc: shouldSupportHealthMonitorFunc,
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Gateway,
				Provider: providers.ProviderKubernetes,
			},
			HealthHandler:                  handlers.NewKubernetesHealthHandler(k8s),
			ResourceHandler:                handlers.NewKubernetesHandler(k8s),
			ShouldSupportHealthMonitorFunc: shouldSupportHealthMonitorFunc,
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.KubernetesHTTPRoute,
				Provider: providers.ProviderKubernetes,
			},
			HealthHandler:                  handlers.NewKubernetesHealthHandler(k8s),
			ResourceHandler:                handlers.NewKubernetesHandler(k8s),
			ShouldSupportHealthMonitorFunc: shouldSupportHealthMonitorFunc,
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.SecretProviderClass,
				Provider: providers.ProviderKubernetes,
			},
			HealthHandler:                  handlers.NewKubernetesHealthHandler(k8s),
			ResourceHandler:                handlers.NewKubernetesHandler(k8s),
			ShouldSupportHealthMonitorFunc: shouldSupportHealthMonitorFunc,
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureStorage,
				Provider: providers.ProviderKubernetes,
			},
			HealthHandler:                  handlers.NewDaprStateStoreAzureStorageHealthHandler(arm, k8s),
			ResourceHandler:                handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s),
			ShouldSupportHealthMonitorFunc: shouldSupportHealthMonitorFunc,
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprComponent,
				Provider: providers.ProviderKubernetes,
			},
			HealthHandler:   handlers.NewKubernetesHealthHandler(k8s),
			ResourceHandler: handlers.NewKubernetesHandler(k8s),
		},
	}

	azureOutputResourceModel := []model.OutputResourceModel{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:          handlers.NewAzureCosmosDBMongoHealthHandler(arm),
			ResourceHandler:        handlers.NewAzureCosmosDBMongoHandler(arm),
			SecretValueTransformer: &mongodbv1alpha3.AzureTransformer{},
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureCosmosAccountMongoHealthHandler(arm),
			ResourceHandler: handlers.NewAzureCosmosAccountHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBSQL,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureCosmosDBSQLHealthHandler(arm),
			ResourceHandler: handlers.NewAzureCosmosDBSQLHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureServiceBusQueue,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureServiceBusQueueHealthHandler(arm),
			ResourceHandler: handlers.NewAzureServiceBusQueueHandler(arm),
			ShouldSupportHealthMonitorFunc: func(resourceType resourcemodel.ResourceType) bool {
				return true
			},
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewDaprPubSubServiceBusHealthHandler(arm, k8s),
			ResourceHandler: handlers.NewDaprPubSubServiceBusHandler(arm, k8s),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureKeyVault,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureKeyVaultHealthHandler(arm),
			ResourceHandler: handlers.NewAzureKeyVaultHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzurePodIdentity,
				Provider: providers.ProviderAzureKubernetesService,
			},
			HealthHandler:   handlers.NewAzurePodIdentityHealthHandler(arm),
			ResourceHandler: handlers.NewAzurePodIdentityHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureSqlServer,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewARMHealthHandler(arm),
			ResourceHandler: handlers.NewARMHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureSqlServerDatabase,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewARMHealthHandler(arm),
			ResourceHandler: handlers.NewARMHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureUserAssignedManagedIdentityHealthHandler(arm),
			ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureRoleAssignmentHealthHandler(arm),
			ResourceHandler: handlers.NewAzureRoleAssignmentHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureKeyVaultSecret,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureKeyVaultSecretHealthHandler(arm),
			ResourceHandler: handlers.NewAzureKeyVaultSecretHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRedis,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureRedisHealthHandler(arm),
			ResourceHandler: handlers.NewAzureRedisHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureFileShare,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureFileShareHealthHandler(arm),
			ResourceHandler: handlers.NewAzureFileShareHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureFileShareStorageAccount,
				Provider: providers.ProviderAzure,
			},
			HealthHandler:   handlers.NewAzureFileShareStorageAccountHealthHandler(arm),
			ResourceHandler: handlers.NewAzureFileShareStorageAccountHandler(arm),
		},
	}

	err := checkForDuplicateRegistrations(radiusResourceModel, outputResourceModel)
	if err != nil {
		return model.ApplicationModel{}, err
	}

	if arm != nil {
		outputResourceModel = append(outputResourceModel, azureOutputResourceModel...)
	}
	return model.NewModel(radiusResourceModel, outputResourceModel, supportedProviders), nil
}

// checkForDuplicateRegistrations checks for duplicate registrations with the same resource type
func checkForDuplicateRegistrations(radiusResources []model.RadiusResourceModel, outputResources []model.OutputResourceModel) error {
	rendererRegistration := make(map[string]int)
	for _, r := range radiusResources {
		rendererRegistration[r.ResourceType]++
		if rendererRegistration[r.ResourceType] > 1 {
			return fmt.Errorf("Multiple resource renderers registered for resource type: %s", r.ResourceType)
		}
	}

	outputResourceHandlerRegistration := make(map[resourcemodel.ResourceType]int)
	for _, o := range outputResources {
		outputResourceHandlerRegistration[o.ResourceType]++
		if outputResourceHandlerRegistration[o.ResourceType] > 1 {
			return fmt.Errorf("Multiple output resource handlers registered for resource type: %s", o.ResourceType)
		}
	}
	return nil
}
