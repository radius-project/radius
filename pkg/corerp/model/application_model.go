// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/mongodatabases"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/rediscaches"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/corerp/renderers/daprextension"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/corerp/renderers/manualscale"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
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
		resourcemodel.ProviderKubernetes: true,
	}
	if arm != nil {
		supportedProviders[resourcemodel.ProviderAzure] = true
		supportedProviders[resourcemodel.ProviderAzureKubernetesService] = true
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
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Service,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Secret,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Gateway,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.KubernetesHTTPRoute,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.SecretProviderClass,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
	}

	azureOutputResourceModel := []OutputResourceModel{
		// Azure CosmosDB and Azure Redis models are consumed by deployment processor to fetch secrets for container dependencies.
		// Any new SecretValueTransformer for a connector should be added here to support connections from container.
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: resourcemodel.ProviderAzure,
			},
			SecretValueTransformer: &mongodatabases.AzureTransformer{},
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRedis,
				Provider: resourcemodel.ProviderAzure,
			},
			SecretValueTransformer: &rediscaches.AzureTransformer{},
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureRoleAssignmentHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureRoleAssignmentHandler(arm),
		},
	}
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
