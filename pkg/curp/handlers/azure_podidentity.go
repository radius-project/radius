// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/workloads"
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

func (pih *azurePodIdentityHandler) GetProperties(resource workloads.WorkloadResource) (map[string]string, error) {
	item, err := convertToUnstructured(resource)
	if err != nil {
		return nil, err
	}

	p := map[string]string{
		"kind":       item.GetKind(),
		"apiVersion": item.GetAPIVersion(),
		"namespace":  item.GetNamespace(),
		"name":       item.GetName(),
	}
	return p, nil
}

func (pih *azurePodIdentityHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// TODO: right now this resource is created during the rendering process :(
	// this should be done here instead when we have built a more mature system.

	return properties, nil
}

func (pih *azurePodIdentityHandler) Delete(ctx context.Context, options DeleteOptions) error {
	// Delete AAD Pod Identity created
	properties := options.Existing.Properties
	podIdentityName := properties[PodIdentityNameKey]
	podidentityCluster := properties[PodIdentityClusterKey]

	mcc := containerservice.NewManagedClustersClient(pih.arm.SubscriptionID)
	mcc.Authorizer = pih.arm.Auth

	// Get the cluster and modify it to remove pod identity
	managedCluster, err := mcc.Get(ctx, pih.arm.ResourceGroup, podidentityCluster)
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

	mcFuture, err := mcc.CreateOrUpdate(ctx, pih.arm.ResourceGroup, podidentityCluster, containerservice.ManagedCluster{
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
	err = pih.deleteManagedIdentity(ctx, *identity.Identity.ResourceID)
	if err != nil {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}

	return nil
}

func (pih *azurePodIdentityHandler) deleteManagedIdentity(ctx context.Context, msiResourceID string) error {
	msiClient := msi.NewUserAssignedIdentitiesClient(pih.arm.SubscriptionID)
	msiClient.Authorizer = pih.arm.Auth
	msiResource, err := azure.ParseResourceID(msiResourceID)
	if err != nil {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}
	resp, err := msiClient.Delete(ctx, pih.arm.ResourceGroup, msiResource.ResourceName)
	if err != nil || (resp.StatusCode != 200 && resp.StatusCode != 204) {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}

	return nil
}
