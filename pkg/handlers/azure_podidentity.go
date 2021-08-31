// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azclients"
	"github.com/Azure/radius/pkg/healthcontract"
)

const (
	PodIdentityNameKey    = "podidentityname"
	PodIdentityClusterKey = "podidentitycluster"
)

func NewAzurePodIdentityHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azurePodIdentityHandler{arm: arm}
}

type azurePodIdentityHandler struct {
	arm armauth.ArmConfig
}

func (handler *azurePodIdentityHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.Existing)

	// if !options.Resource.Deployed {
	// TODO: right now this resource is already deployed during the rendering process :(
	// this should be done here instead when we have built a more mature system.
	// }

	return properties, nil
}

func (handler *azurePodIdentityHandler) Delete(ctx context.Context, options DeleteOptions) error {
	// Delete AAD Pod Identity created
	properties := options.Existing.Properties
	podIdentityName := properties[PodIdentityNameKey]
	podidentityCluster := properties[PodIdentityClusterKey]

	// Conceptually this resource is always 'managed'

	mcc := azclients.NewManagedClustersClient(handler.arm.SubscriptionID, handler.arm.Auth)

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
	msiClient := azclients.NewUserAssignedIdentitiesClient(handler.arm.SubscriptionID, handler.arm.Auth)

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

func NewAzurePodIdentityHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azurePodIdentityHealthHandler{arm: arm}
}

type azurePodIdentityHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azurePodIdentityHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
