// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/msi/mgmt/msi"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
)

const (
	UserAssignedIdentityNameKey        = "userassignedidentityname"
	UserAssignedIdentityIDKey          = "userassignedidentityid"
	UserAssignedIdentityPrincipalIDKey = "userassignedidentityprincipalid"
	UserAssignedIdentityClientIDKey    = "userassignedidentityclientid"
)

// NewAzureUserAssignedManagedIdentityHandler initializes a new handler for resources of kind UserAssignedManagedIdentity
func NewAzureUserAssignedManagedIdentityHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureUserAssignedManagedIdentityHandler{arm: arm}
}

type azureUserAssignedManagedIdentityHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureUserAssignedManagedIdentityHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := radlogger.GetLogger(ctx)
	properties := mergeProperties(*options.Resource, options.Existing)

	rgLocation, err := clients.GetResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	identityName := properties[UserAssignedIdentityNameKey]
	msiClient := clients.NewUserAssignedIdentitiesClient(handler.arm.SubscriptionID, handler.arm.Auth)
	identity, err := msiClient.CreateOrUpdate(context.Background(), handler.arm.ResourceGroup, identityName, msi.Identity{
		Location: to.StringPtr(*rgLocation),
		Tags:     keys.MakeTagsForRadiusComponent(options.Application, options.Component),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create user assigned managed identity: %w", err)
	}
	properties[UserAssignedIdentityIDKey] = *identity.ID
	properties[UserAssignedIdentityPrincipalIDKey] = identity.PrincipalID.String()
	properties[UserAssignedIdentityClientIDKey] = identity.ClientID.String()

	options.Resource.Info = outputresource.ARMInfo{
		ID:           *identity.ID,
		ResourceType: *identity.Type,
		APIVersion:   msi.Version(),
	}

	logger.WithValues(
		radlogger.LogFieldResourceID, *identity.ID,
		radlogger.LogFieldLocalID, outputresource.LocalIDUserAssignedManagedIdentityKV).Info("Created managed identity for KeyVault access")

	return properties, nil
}

func (handler *azureUserAssignedManagedIdentityHandler) Delete(ctx context.Context, options DeleteOptions) error {
	// TODO: right now this resource is deleted in a different handler :(
	// this should be done here instead when we have built a more mature system.

	return nil
}

func NewAzureUserAssignedManagedIdentityHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureUserAssignedManagedIdentityHealthHandler{arm: arm}
}

type azureUserAssignedManagedIdentityHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureUserAssignedManagedIdentityHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
