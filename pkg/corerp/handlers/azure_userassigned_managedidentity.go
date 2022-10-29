// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

const (
	UserAssignedIdentityNameKey        = "userassignedidentityname"
	UserAssignedIdentityIDKey          = "userassignedidentityid"
	UserAssignedIdentityPrincipalIDKey = "userassignedidentityprincipalid"
	UserAssignedIdentityClientIDKey    = "userassignedidentityclientid"
	UserAssignedIdentityTenantIDKey    = "userassignedidentitytenantid"
	UserAssignedIdentityScope          = "userassignedidentityscope"
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

	rgLocation, err := clients.GetResourceGroupLocation(ctx, *handler.arm, handler.SubscriptionID, handler.ResourceGroup)
	if err != nil {
		return properties, err
	}

	identityName := properties[UserAssignedIdentityNameKey]
	msiClient := clients.NewUserAssignedIdentitiesClient(handler.SubscriptionID, handler.arm.Auth)
	identity, err := msiClient.CreateOrUpdate(context.Background(), handler.ResourceGroup, identityName, msi.Identity{
		Location: to.Ptr(*rgLocation),
	})
	if err != nil {
		return properties, fmt.Errorf("failed to create user assigned managed identity: %w", err)
	}

	properties[UserAssignedIdentityIDKey] = *identity.ID
	properties[UserAssignedIdentityPrincipalIDKey] = identity.PrincipalID.String()
	properties[UserAssignedIdentityClientIDKey] = identity.ClientID.String()

	options.Resource.Identity = resourcemodel.NewARMIdentity(&options.Resource.ResourceType, properties[UserAssignedIdentityIDKey], clients.GetAPIVersionFromUserAgent(msi.UserAgent()))
	logger.WithValues(
		radlogger.LogFieldResourceID, *identity.ID,
		radlogger.LogFieldLocalID, outputresource.LocalIDUserAssignedManagedIdentity).Info("Created managed identity for KeyVault access")

	return properties, nil
}

func (handler *azureUserAssignedManagedIdentityHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	// TODO: right now this resource is deleted in a different handler :(
	// this should be done here instead when we have built a more mature system.
	return nil
}
