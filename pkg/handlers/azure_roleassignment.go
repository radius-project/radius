// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/authorization/mgmt/authorization"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/azure/roleassignment"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcemodel"
)

const (
	RoleNameKey = "rolename"
)

// NewAzureRoleAssignmentHandler initializes a new handler for resources of kind RoleAssignment
func NewAzureRoleAssignmentHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureRoleAssignmentHandler{arm: arm}
}

type azureRoleAssignmentHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureRoleAssignmentHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := radlogger.GetLogger(ctx)
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	roleName := properties[RoleNameKey]
	keyVaultName := properties[KeyVaultNameKey]

	// Get dependencies
	managedIdentityProperties := map[string]string{}
	for _, resource := range options.Dependencies {
		if resource.LocalID == outputresource.LocalIDUserAssignedManagedIdentityKV {
			managedIdentityProperties = resource.Properties
		}
	}

	if properties, ok := options.DependencyProperties[outputresource.LocalIDUserAssignedManagedIdentityKV]; ok {
		managedIdentityProperties = properties
	}

	if len(managedIdentityProperties) == 0 {
		return nil, errors.New("missing dependency: a user assigned identity is required to create role assignment")
	}

	keyVaultClient := clients.NewVaultsClient(handler.arm.SubscriptionID, handler.arm.Auth)
	keyVault, err := keyVaultClient.Get(ctx, handler.arm.ResourceGroup, keyVaultName)
	if err != nil {
		return nil, fmt.Errorf("failed to get key vault information: %w", err)
	}

	// Assign Key Vault Secrets User role to grant managed identity read-only access to the keyvault for secrets.
	// Assign Key Vault Crypto User role to grant managed identity permissions to perform operations using encryption keys.
	roleAssignment, err := roleassignment.Create(ctx, handler.arm.Auth, handler.arm.SubscriptionID, handler.arm.ResourceGroup, managedIdentityProperties[UserAssignedIdentityPrincipalIDKey], *keyVault.ID, roleName)
	if err != nil {
		return nil,
			fmt.Errorf("Failed to assign '%s' role to the managed identity '%s' within keyvault '%s' scope : %w", roleName, managedIdentityProperties[UserAssignedIdentityIDKey], keyVaultName, err)
	}
	logger.WithValues(radlogger.LogFieldLocalID, outputresource.LocalIDRoleAssignmentKVKeys).Info(fmt.Sprintf("Created %s role assignment for %s to access %s", roleName, managedIdentityProperties[UserAssignedIdentityIDKey], *keyVault.ID))

	options.Resource.Identity = resourcemodel.NewARMIdentity(*roleAssignment.ID, clients.GetAPIVersionFromUserAgent(authorization.UserAgent()))
	return properties, nil
}

func (handler *azureRoleAssignmentHandler) Delete(ctx context.Context, options DeleteOptions) error {
	// TODO: right now this resource is deleted in a different handler :(
	// this should be done here instead when we have built a more mature system.

	return nil
}

func NewAzureRoleAssignmentHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureRoleAssignmentHealthHandler{arm: arm}
}

type azureRoleAssignmentHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureRoleAssignmentHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
