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

	// RoleAssignmentTargetKey is used to pass the Resource ID of the target resource.
	RoleAssignmentTargetKey = "roleassignmenttarget"
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
	targetID := properties[RoleAssignmentTargetKey]

	// Get dependencies
	managedIdentityProperties := map[string]string{}
	for _, resource := range options.Dependencies {
		if resource.LocalID == outputresource.LocalIDUserAssignedManagedIdentity {
			managedIdentityProperties = resource.Properties
		}
	}

	if properties, ok := options.DependencyProperties[outputresource.LocalIDUserAssignedManagedIdentity]; ok {
		managedIdentityProperties = properties
	}

	if len(managedIdentityProperties) == 0 {
		return nil, errors.New("missing dependency: a user assigned identity is required to create role assignment")
	}

	// Assign Key Vault Secrets User role to grant managed identity read-only access to the keyvault for secrets.
	// Assign Key Vault Crypto User role to grant managed identity permissions to perform operations using encryption keys.
	roleAssignment, err := roleassignment.Create(ctx, handler.arm.Auth, handler.arm.SubscriptionID, handler.arm.ResourceGroup, managedIdentityProperties[UserAssignedIdentityPrincipalIDKey], targetID, roleName)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to assign '%s' role to the managed identity '%s' within resource '%s' scope : %w",
			roleName,
			managedIdentityProperties[UserAssignedIdentityIDKey],
			targetID,
			err)
	}
	logger.WithValues(radlogger.LogFieldLocalID, outputresource.LocalIDRoleAssignmentKVKeys).Info(fmt.Sprintf("Created %s role assignment for %s to access %s", roleName, managedIdentityProperties[UserAssignedIdentityIDKey], targetID))

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
