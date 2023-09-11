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

	"github.com/radius-project/radius/pkg/azure/armauth"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/handlers"
	"github.com/radius-project/radius/pkg/corerp/renderers/container"
	azcontainer "github.com/radius-project/radius/pkg/corerp/renderers/container/azure"
	"github.com/radius-project/radius/pkg/corerp/renderers/daprextension"
	"github.com/radius-project/radius/pkg/corerp/renderers/gateway"
	"github.com/radius-project/radius/pkg/corerp/renderers/httproute"
	"github.com/radius-project/radius/pkg/corerp/renderers/kubernetesmetadata"
	"github.com/radius-project/radius/pkg/corerp/renderers/manualscale"
	"github.com/radius-project/radius/pkg/corerp/renderers/volume"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	resources_azure "github.com/radius-project/radius/pkg/ucp/resources/azure"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// AnyResourceType is used to designate a resource handler that can handle any type that belongs to a provider. AnyResourceType
	// should only be used to register handlers, and not as part of an output resource.
	AnyResourceType = "*"
)

// NewApplicationModel configures RBAC support on connections based on connection kind, configures the providers supported by the appmodel,
// registers the renderers and handlers for various resources, and checks for duplicate registrations.
func NewApplicationModel(arm *armauth.ArmConfig, k8sClient client.Client, k8sClientSet kubernetes.Interface, discoveryClient discovery.ServerResourcesInterface, k8sDynamicClientSet dynamic.Interface) (ApplicationModel, error) {
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
			// More information can be found here: https://github.com/radius-project/radius/issues/1321
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
				Type:     AnyResourceType,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceHandler: handlers.NewKubernetesHandler(k8sClient, k8sClientSet, discoveryClient, k8sDynamicClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resources_kubernetes.ResourceTypeSecretProviderClass,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceTransformer: azcontainer.TransformSecretProviderClass,
			ResourceHandler:     handlers.NewKubernetesHandler(k8sClient, k8sClientSet, discoveryClient, k8sDynamicClientSet),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resources_kubernetes.ResourceTypeServiceAccount,
				Provider: resourcemodel.ProviderKubernetes,
			},
			ResourceTransformer: azcontainer.TransformFederatedIdentitySA,
			ResourceHandler:     handlers.NewKubernetesHandler(k8sClient, k8sClientSet, discoveryClient, k8sDynamicClientSet),
		},
	}

	azureOutputResourceModel := []OutputResourceModel{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentity,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentityFederatedIdentityCredential,
				Provider: resourcemodel.ProviderAzure,
			},
			ResourceHandler: handlers.NewAzureFederatedIdentity(arm),
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resources_azure.ResourceTypeAuthorizationRoleAssignment,
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
