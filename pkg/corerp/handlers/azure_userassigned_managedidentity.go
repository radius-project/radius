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
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

const (
	UserAssignedIdentityNameKey        = "userassignedidentityname"
	UserAssignedIdentityIDKey          = "userassignedidentityid"
	UserAssignedIdentityPrincipalIDKey = "userassignedidentityprincipalid"
	UserAssignedIdentityClientIDKey    = "userassignedidentityclientid"
)

// NewAzureUserAssignedManagedIdentityHandler initializes a new handler for resources of kind UserAssignedManagedIdentity
func NewAzureUserAssignedManagedIdentityHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureUserAssignedManagedIdentityHandler{arm: arm}
}

type azureUserAssignedManagedIdentityHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureUserAssignedManagedIdentityHandler) Put(ctx context.Context, resource *outputresource.OutputResource) error {
	logger := radlogger.GetLogger(ctx)
	resource_identity, err := handler.GetResourceIdentity(ctx, *resource)
	if err != nil {
		return err
	}

	resource.Identity = resource_identity
	id := resource_identity.Data.(resourcemodel.ARMIdentity)
	logger.WithValues(
		radlogger.LogFieldResourceID, id,
		radlogger.LogFieldLocalID, outputresource.LocalIDUserAssignedManagedIdentity).Info("Created managed identity for KeyVault access")

	return nil
}

func (handler *azureUserAssignedManagedIdentityHandler) GetResourceIdentity(ctx context.Context, resource outputresource.OutputResource) (resourcemodel.ResourceIdentity, error) {
	properties, err := handler.GetResourceNativeIdentityKeyProperties(ctx, resource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, err
	}

	identity := resourcemodel.NewARMIdentity(&resource.ResourceType, properties[UserAssignedIdentityIDKey], clients.GetAPIVersionFromUserAgent(msi.UserAgent()))

	return identity, nil
}

func (handler *azureUserAssignedManagedIdentityHandler) GetResourceNativeIdentityKeyProperties(ctx context.Context, resource outputresource.OutputResource) (map[string]string, error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	rgLocation, err := clients.GetResourceGroupLocation(ctx, *handler.arm)
	if err != nil {
		return properties, err
	}

	identityName := properties[UserAssignedIdentityNameKey]
	msiClient := clients.NewUserAssignedIdentitiesClient(handler.arm.SubscriptionID, handler.arm.Auth)
	identity, err := msiClient.CreateOrUpdate(context.Background(), handler.arm.ResourceGroup, identityName, msi.Identity{
		Location: to.StringPtr(*rgLocation),
	})
	if err != nil {
		return properties, fmt.Errorf("failed to create user assigned managed identity: %w", err)
	}
	properties[UserAssignedIdentityIDKey] = *identity.ID
	properties[UserAssignedIdentityPrincipalIDKey] = identity.PrincipalID.String()
	properties[UserAssignedIdentityClientIDKey] = identity.ClientID.String()

	return properties, nil
}

func (handler *azureUserAssignedManagedIdentityHandler) Delete(ctx context.Context, resource outputresource.OutputResource) error {
	return nil
}
