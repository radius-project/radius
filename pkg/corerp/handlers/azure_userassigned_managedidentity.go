// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
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
	logger := radlogger.GetLogger(ctx)

	properties, ok := options.Resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	identityName, err := GetString(properties, UserAssignedIdentityNameKey)
	if err != nil {
		return nil, err
	}

	subID, err := GetString(properties, UserAssignedIdentitySubscriptionID)
	if err != nil {
		return nil, err
	}

	rgName, err := GetString(properties, UserAssignedIdentityResourceGroup)
	if err != nil {
		return nil, err
	}

	rgLocation, err := clients.GetResourceGroupLocation(ctx, *handler.arm, subID, rgName)
	if err != nil {
		return properties, err
	}

	msiClient := clients.NewUserAssignedIdentitiesClient(subID, handler.arm.Auth)
	identity, err := msiClient.CreateOrUpdate(ctx, rgName, identityName, msi.Identity{Location: rgLocation})
	if err != nil {
		return properties, fmt.Errorf("failed to create user assigned managed identity: %w", err)
	}

	properties[UserAssignedIdentityIDKey] = to.String(identity.ID)
	properties[UserAssignedIdentityPrincipalIDKey] = identity.PrincipalID.String()
	properties[UserAssignedIdentityClientIDKey] = identity.ClientID.String()
	properties[UserAssignedIdentityTenantIDKey] = identity.TenantID.String()

	options.Resource.Identity = resourcemodel.NewARMIdentity(&options.Resource.ResourceType, properties[UserAssignedIdentityIDKey], clients.GetAPIVersionFromUserAgent(msi.UserAgent()))
	logger.WithValues(
		radlogger.LogFieldResourceID, *identity.ID,
		radlogger.LogFieldLocalID, outputresource.LocalIDUserAssignedManagedIdentity).Info("Created managed identity for KeyVault access")

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

	msiClient := clients.NewUserAssignedIdentitiesClient(parsed.FindScope(resources.SubscriptionsSegment), handler.arm.Auth)
	_, err = msiClient.Delete(ctx, parsed.FindScope(resources.ResourceGroupsSegment), parsed.Name())

	if err != nil {
		return fmt.Errorf("failed to delete user assigned managed identity: %w", err)
	}

	return nil
}
