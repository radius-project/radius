// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

const (
	PodIdentityNameKey = "podidentityname"
	PodNamespaceKey    = "podnamespace"
)

func NewAzurePodIdentityHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azurePodIdentityHandler{arm: arm}
}

type azurePodIdentityHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azurePodIdentityHandler) Put(ctx context.Context, resource *outputresource.OutputResource) error {
	logger := radlogger.GetLogger(ctx)
	properties, err := handler.GetResourceNativeIdentityKeyProperties(ctx, *resource)
	if err != nil {
		return err
	}

	// Get dependencies
	managedIdentityProperties := map[string]string{}
	if prop, ok := properties[outputresource.LocalIDUserAssignedManagedIdentity]; ok {
		managedIdentityProperties[outputresource.LocalIDUserAssignedManagedIdentity] = prop
	}

	if len(managedIdentityProperties) == 0 {
		return errors.New("missing dependency: a user assigned identity is required to create role assignment")
	}

	// Get AKS cluster name in current resource group and update it to add pod identity
	clustersClient := clients.NewManagedClustersClient(handler.arm.K8sSubscriptionID, handler.arm.Auth)
	managedCluster, err := clustersClient.Get(ctx, handler.arm.K8sResourceGroup, handler.arm.K8sClusterName)
	if err != nil {
		return fmt.Errorf("failed to get managed cluster details for cluster %s in the resource group %s: %w", handler.arm.K8sClusterName, handler.arm.K8sResourceGroup, err)
	}

	managedCluster.PodIdentityProfile.Enabled = to.BoolPtr(true)
	managedCluster.PodIdentityProfile.AllowNetworkPluginKubenet = to.BoolPtr(false)

	podIdentityName := properties[PodIdentityNameKey]
	podNamespace := properties[PodNamespaceKey]
	clusterPodIdentity := containerservice.ManagedClusterPodIdentity{
		Name:      &podIdentityName,
		Namespace: &podNamespace, // Note: The pod identity namespace specified here has to match the namespace in which the application is deployed
		Identity: &containerservice.UserAssignedIdentity{
			ResourceID: to.StringPtr(managedIdentityProperties[UserAssignedIdentityIDKey]),
			ClientID:   to.StringPtr(managedIdentityProperties[UserAssignedIdentityClientIDKey]),
			ObjectID:   to.StringPtr(managedIdentityProperties[UserAssignedIdentityPrincipalIDKey]),
		},
	}

	var identities []containerservice.ManagedClusterPodIdentity
	if managedCluster.ManagedClusterProperties.PodIdentityProfile.UserAssignedIdentities != nil {
		identities = *managedCluster.PodIdentityProfile.UserAssignedIdentities
	}
	identities = append(identities, clusterPodIdentity)

	// Handling of eventual consistency here can really use some work and improvements, more details:
	// https://github.com/project-radius/radius/issues/1010
	// https://github.com/project-radius/radius/issues/660
	// For now just moving it over as is from renderer to limit the scope of changes.
	MaxRetries := 100
	var resultFuture containerservice.ManagedClustersCreateOrUpdateFuture
	for i := 0; i <= MaxRetries; i++ {
		// Retry to wait for the managed identity to propagate
		if i >= MaxRetries {
			return fmt.Errorf("failed to add pod identity on the cluster %s: %w", handler.arm.K8sClusterName, err)
		}

		resultFuture, err = clustersClient.CreateOrUpdate(ctx, handler.arm.K8sResourceGroup, handler.arm.K8sClusterName, containerservice.ManagedCluster{
			ManagedClusterProperties: &containerservice.ManagedClusterProperties{
				PodIdentityProfile: &containerservice.ManagedClusterPodIdentityProfile{
					Enabled:                   to.BoolPtr(true),
					AllowNetworkPluginKubenet: to.BoolPtr(false),
					UserAssignedIdentities:    &identities,
				},
			},
			Location: managedCluster.Location,
		})

		if err == nil {
			break
		}

		// Check the error and determine if it is retryable
		detailed, ok := clients.ExtractDetailedError(err)
		if !ok {
			return err
		}

		// Sometimes, the managed identity takes a while to propagate and the pod identity creation fails with status code = 0
		// For other reasons, fail
		if detailed.StatusCode != 0 {
			return fmt.Errorf("failed to add pod identity on the cluster with error: %v, status code: %v", detailed.Message, detailed.StatusCode)
		}

		logger.V(radlogger.Verbose).Info("failed to add pod identity. Retrying...")
		time.Sleep(5 * time.Second)
		continue
	}

	err = resultFuture.WaitForCompletionRef(ctx, clustersClient.Client)
	if err != nil {
		return fmt.Errorf("failed to add pod identity on the cluster: %w", err)
	}

	resource.Identity = resourcemodel.ResourceIdentity{
		ResourceType: &resourcemodel.ResourceType{
			Type:     resourcekinds.AzurePodIdentity,
			Provider: providers.ProviderAzureKubernetesService,
		},
		Data: resourcemodel.AADPodIdentityIdentity{
			AKSClusterName: handler.arm.K8sClusterName,
			Name:           podIdentityName,
			Namespace:      podNamespace,
		},
	}

	return nil
}

func (handler *azurePodIdentityHandler) GetResourceNativeIdentityKeyProperties(ctx context.Context, resource outputresource.OutputResource) (map[string]string, error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	if handler.arm.K8sSubscriptionID == "" || handler.arm.K8sResourceGroup == "" || handler.arm.K8sClusterName == "" {
		return nil, errors.New("pod identity is not supported because the RP is not configured for AKS")
	}

	// Get dependencies
	managedIdentityProperties := map[string]string{}
	if prop, ok := properties[outputresource.LocalIDUserAssignedManagedIdentity]; ok {
		managedIdentityProperties[outputresource.LocalIDUserAssignedManagedIdentity] = prop
	}

	if len(managedIdentityProperties) == 0 {
		return properties, errors.New("missing dependency: a user assigned identity is required to create role assignment")
	}

	return properties, nil
}

func (handler *azurePodIdentityHandler) GetResourceIdentity(ctx context.Context, resource outputresource.OutputResource) (resourcemodel.ResourceIdentity, error) {
	properties, err := handler.GetResourceNativeIdentityKeyProperties(ctx, resource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, err
	}

	podIdentityName := properties[PodIdentityNameKey]
	podNamespace := properties[PodNamespaceKey]
	identity := resourcemodel.ResourceIdentity{
		ResourceType: &resourcemodel.ResourceType{
			Type:     resourcekinds.AzurePodIdentity,
			Provider: providers.ProviderAzureKubernetesService,
		},
		Data: resourcemodel.AADPodIdentityIdentity{
			AKSClusterName: handler.arm.K8sClusterName,
			Name:           podIdentityName,
			Namespace:      podNamespace,
		},
	}

	return identity, nil
}

func (handler *azurePodIdentityHandler) Delete(ctx context.Context, resource outputresource.OutputResource) error {
	podidentityCluster, podIdentityName, _, err := resource.Identity.RequireAADPodIdentity()
	if err != nil {
		return err
	}
	// Conceptually this resource is always 'managed'

	mcc := clients.NewManagedClustersClient(handler.arm.SubscriptionID, handler.arm.Auth)

	// Get the cluster and modify it to remove pod identity
	managedCluster, err := mcc.Get(ctx, handler.arm.K8sResourceGroup, podidentityCluster)
	if err != nil {
		return fmt.Errorf("failed to get managed cluster: %w", err)
	}

	var identities []containerservice.ManagedClusterPodIdentity
	if managedCluster.ManagedClusterProperties.PodIdentityProfile.UserAssignedIdentities == nil {
		// Pod identity does not exist
		return nil
	}

	identities = *managedCluster.PodIdentityProfile.UserAssignedIdentities

	var i int
	var identity containerservice.ManagedClusterPodIdentity
	for i, identity = range *managedCluster.ManagedClusterProperties.PodIdentityProfile.UserAssignedIdentities {
		if *identity.Name == podIdentityName {
			break
		}
	}

	// Remove the pod identity at the matching index
	identities = append(identities[:i], identities[i+1:]...)

	mcFuture, err := mcc.CreateOrUpdate(ctx, handler.arm.K8sResourceGroup, podidentityCluster, containerservice.ManagedCluster{
		ManagedClusterProperties: &containerservice.ManagedClusterProperties{
			PodIdentityProfile: &containerservice.ManagedClusterPodIdentityProfile{
				Enabled:                   to.BoolPtr(true),
				AllowNetworkPluginKubenet: to.BoolPtr(false),
				UserAssignedIdentities:    &identities,
			},
		},
		Location: managedCluster.Location,
	})

	if err != nil {
		return fmt.Errorf("failed to delete pod identity on the cluster: %w", err)
	}

	err = mcFuture.WaitForCompletionRef(ctx, mcc.Client)
	if err != nil {
		return fmt.Errorf("failed to delete pod identity on the cluster: %w", err)
	}

	// Delete the managed identity
	err = handler.deleteManagedIdentity(ctx, *identity.Identity.ResourceID)
	if err != nil {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}

	return nil
}

func (handler *azurePodIdentityHandler) deleteManagedIdentity(ctx context.Context, msiResourceID string) error {
	msiClient := clients.NewUserAssignedIdentitiesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	msiResource, err := azure.ParseResourceID(msiResourceID)
	if err != nil {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}
	resp, err := msiClient.Delete(ctx, handler.arm.ResourceGroup, msiResource.ResourceName)
	if err != nil || (resp.StatusCode != 200 && resp.StatusCode != 204) {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}

	return nil
}
