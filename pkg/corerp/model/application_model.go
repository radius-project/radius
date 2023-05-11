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

package model

import (
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	azcontainer "github.com/project-radius/radius/pkg/corerp/renderers/container/azure"
	"github.com/project-radius/radius/pkg/corerp/renderers/daprextension"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/corerp/renderers/kubernetesmetadata"
	"github.com/project-radius/radius/pkg/corerp/renderers/manualscale"
	"github.com/project-radius/radius/pkg/corerp/renderers/volume"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"

	"github.com/project-radius/radius/pkg/resourcekinds"
	"k8s.io/client-go/kubernetes"
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
			LocalID: rpv1.LocalIDKeyVault,
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
	}

	radiusResourceModel := []RadiusResourceModel{
		{
			ResourceType: container.ResourceType,
			Renderer: &kubernetesmetadata.Renderer{
				Inner: &manualscale.Renderer{
					Inner: &daprextension.Renderer{
						Inner: &container.Renderer{
							RoleAssignmentMap: roleAssignmentMap,
						},
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
		{
			ResourceType: volume.ResourceType,
			Renderer:     volume.NewRenderer(arm),
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
				Type:     resourcekinds.Volume,
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
			ResourceTransformer: azcontainer.TransformSecretProviderClass,
			ResourceHandler:     handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.ServiceAccount,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceTransformer: azcontainer.TransformFederatedIdentitySA,
			ResourceHandler:     handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.KubernetesRole,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.KubernetesRoleBinding,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet),
		},
	}

	azureOutputResourceModel := []OutputResourceModel{
		// Azure CosmosDB and Azure Redis models are consumed by deployment processor to fetch secrets for container dependencies.
		// Any new SecretValueTransformer for a link should be added here to support connections from container.
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureFederatedIdentity,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureFederatedIdentity(arm),
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
