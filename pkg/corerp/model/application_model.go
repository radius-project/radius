// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/corerp/renderers/daprextension"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/corerp/renderers/manualscale"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"k8s.io/client-go/kubernetes"

	"github.com/project-radius/radius/pkg/resourcekinds"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewApplicationModel(arm *armauth.ArmConfig, k8sClient client.Client, k8sClientSet kubernetes.Interface) (ApplicationModel, error) {
	// Configure RBAC support on connections based connection kind.
	// Role names can be user input or default roles assigned by Radius.
	// Leave RoleNames field empty if no default roles are supported for a connection kind.
	//
	// For a primer on how to read this data, see the KeyVault case.
	roleAssignmentMap := map[datamodel.IAMKind]container.RoleAssignmentData{

		// Example of how to read this data:
		//
		// For a KeyVault connection...
		// - Look up the dependency based on the connection.Source (azure.com.KeyVault)
		// - Find the output resource matching LocalID of that dependency (Microsoft.KeyVault/vaults)
		// - Apply the roles in RoleNames (Key Vault Secrets User, Key Vault Crypto User)
		datamodel.KindAzureComKeyVault: {
			LocalID: outputresource.LocalIDKeyVault,
			RoleNames: []string{
				"Key Vault Secrets User",
				"Key Vault Crypto User",
			},
		},
		datamodel.KindAzure: {
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

	radiusResourceModel := []RadiusResourceModel{
		{
			ResourceType: container.ResourceType,
			Renderer: &manualscale.Renderer{
				Inner: &daprextension.Renderer{
					Inner: &container.Renderer{
						RoleAssignmentMap: roleAssignmentMap,
					},
				},
			},
		},
		{
			ResourceType: httproute.ResourceType,
			Renderer:     &httproute.Renderer{},
		},
		{
			ResourceType: gateway.ResourceType,
			Renderer:     &gateway.Renderer{},
		},
	}

	outputResourceModel := []OutputResourceModel{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Kubernetes,
				Provider: providers.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: providers.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Service,
				Provider: providers.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Secret,
				Provider: providers.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Gateway,
				Provider: providers.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.KubernetesHTTPRoute,
				Provider: providers.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.SecretProviderClass,
				Provider: providers.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
	}

	// TODO: Adding handlers next after this changelist
	azureOutputResourceModel := []OutputResourceModel{
		// HACK adding CosmosDB because SecretValueTransformer is custom.
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: providers.ProviderAzure,
			},
			ResourceHandler:        handlers.NewAzureCosmosDBMongoHandler(arm),
			SecretValueTransformer: &renderers.AzureTransformer{},
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: providers.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: providers.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureRoleAssignmentHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: providers.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: providers.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureRoleAssignmentHandler(arm),
		},
	}
	/* 	azureOutputResourceModel := []OutputResourceModel{
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureCosmosDBMongo,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler:        handlers.NewAzureCosmosDBMongoHandler(arm),
	   			SecretValueTransformer: &mongodbv1alpha3.AzureTransformer{},
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.DaprStateStoreAzureStorage,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureCosmosAccount,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewAzureCosmosAccountHandler(arm),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureCosmosDBSQL,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewAzureCosmosDBSQLHandler(arm),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewDaprPubSubServiceBusHandler(arm, k8s),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzurePodIdentity,
	   				Provider: providers.ProviderAzureKubernetesService,
	   			},
	   			ResourceHandler: handlers.NewAzurePodIdentityHandler(arm),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureSqlServer,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewARMHandler(arm),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureSqlServerDatabase,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewARMHandler(arm),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureRoleAssignment,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewAzureRoleAssignmentHandler(arm),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureRedis,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewAzureRedisHandler(arm),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureFileShare,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewAzureFileShareHandler(arm),
	   		},
	   		{
	   			ResourceType: resourcemodel.ResourceType{
	   				Type:     resourcekinds.AzureFileShareStorageAccount,
	   				Provider: providers.ProviderAzure,
	   			},
	   			ResourceHandler: handlers.NewAzureFileShareStorageAccountHandler(arm),
	   		},
	   	}
	*/
	err := checkForDuplicateRegistrations(radiusResourceModel, outputResourceModel)
	if err != nil {
		return ApplicationModel{}, err
	}

	if arm != nil {
		outputResourceModel = append(outputResourceModel, azureOutputResourceModel...)
	}
	return NewModel(radiusResourceModel, outputResourceModel, supportedProviders), nil
}

// checkForDuplicateRegistrations checks for duplicate registrations with the same resource type
func checkForDuplicateRegistrations(radiusResources []RadiusResourceModel, outputResources []OutputResourceModel) error {
	rendererRegistration := make(map[string]int)
	for _, r := range radiusResources {
		rendererRegistration[r.ResourceType]++
		if rendererRegistration[r.ResourceType] > 1 {
			return fmt.Errorf("multiple resource renderers registered for resource type: %s", r.ResourceType)
		}
	}

	outputResourceHandlerRegistration := make(map[resourcemodel.ResourceType]int)
	for _, o := range outputResources {
		outputResourceHandlerRegistration[o.ResourceType]++
		if outputResourceHandlerRegistration[o.ResourceType] > 1 {
			return fmt.Errorf("multiple output resource handlers registered for resource type: %s", o.ResourceType)
		}
	}
	return nil
}
