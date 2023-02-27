// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

const (
	IdentityProperties                 = "identityproperties"
	UserAssignedIdentityNameKey        = "userassignedidentityname"
	UserAssignedIdentityIDKey          = "userassignedidentityid"
	UserAssignedIdentityPrincipalIDKey = "userassignedidentityprincipalid"
	UserAssignedIdentityClientIDKey    = "userassignedidentityclientid"
	UserAssignedIdentityTenantIDKey    = "userassignedidentitytenantid"
	UserAssignedIdentitySubscriptionID = "userassignedidentitysubscriptionid"
	UserAssignedIdentityResourceGroup  = "userassignedidentityresourcegroup"
)

// NewAzureUserAssignedManagedIdentityHandler initializes a new handler for resources of kind UserAssignedManagedIdentity
func NewAzureUserAssignedManagedIdentityHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureUserAssignedManagedIdentityHandler{arm: arm}
}

type azureUserAssignedManagedIdentityHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureUserAssignedManagedIdentityHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := ucplog.FromContext(ctx)

	properties, ok := options.Resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	identityName, err := GetMapValue[string](properties, UserAssignedIdentityNameKey)
	if err != nil {
		return nil, err
	}

	subID, err := GetMapValue[string](properties, UserAssignedIdentitySubscriptionID)
	if err != nil {
		return nil, err
	}

	rgName, err := GetMapValue[string](properties, UserAssignedIdentityResourceGroup)
	if err != nil {
		return nil, err
	}

	rgLocation, err := clientv2.GetResourceGroupLocation(ctx, subID, rgName, &handler.arm.ClientOptions)
	if err != nil {
		return properties, err
	}

	resourceLocation := *rgLocation

	// Federated identity is in preview. Some region doesn't support federated identity.
	// Reference: https://learn.microsoft.com/en-us/azure/active-directory/develop/workload-identity-federation-considerations#unsupported-regions-user-assigned-managed-identities
	// TODO: Remove when all regions support Federated identity.
	if !isFederatedIdentitySupported(resourceLocation) {
		return nil, fmt.Errorf("azure federated identity does not support %s region now. unsupported regions: %q", resourceLocation, federatedUnsupportedRegions)
	}

	msiClient, err := clientv2.NewUserAssignedIdentityClient(subID, &handler.arm.ClientOptions)
	if err != nil {
		return nil, err
	}

	identity, err := msiClient.CreateOrUpdate(ctx, rgName, identityName, armmsi.Identity{Location: &resourceLocation}, nil)
	if err != nil {
		return properties, fmt.Errorf("failed to create user assigned managed identity: %w", err)
	}

	properties[UserAssignedIdentityIDKey] = to.String(identity.ID)
	properties[UserAssignedIdentityPrincipalIDKey] = to.String(identity.Properties.PrincipalID)
	properties[UserAssignedIdentityClientIDKey] = to.String(identity.Properties.ClientID)
	properties[UserAssignedIdentityTenantIDKey] = to.String(identity.Properties.TenantID)

	options.Resource.Identity = resourcemodel.NewARMIdentity(&options.Resource.ResourceType, properties[UserAssignedIdentityIDKey], clientv2.MSIClientAPIVersion)
	logger.Info("Created managed identity for KeyVault access", ucplog.Attributes(ctx, logging.LogFieldLocalID, rpv1.LocalIDUserAssignedManagedIdentity))

	return properties, nil
}

func (handler *azureUserAssignedManagedIdentityHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	msiResourceID, _, err := options.Resource.Identity.RequireARM()
	if err != nil {
		return err
	}

	parsed, err := resources.ParseResource(msiResourceID)
	if err != nil {
		return err
	}

	subscriptionID := parsed.FindScope(resources.SubscriptionsSegment)

	msiClient, err := clientv2.NewUserAssignedIdentityClient(subscriptionID, &handler.arm.ClientOptions)
	if err != nil {
		return err
	}

	_, err = msiClient.Delete(ctx, parsed.FindScope(resources.ResourceGroupsSegment), parsed.Name(), nil)

	if err != nil {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}

	return nil
}
