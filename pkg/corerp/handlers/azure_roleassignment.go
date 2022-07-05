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
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/roleassignment"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

const (
	RoleNameKey = "rolename"

	// RoleAssignmentScope is used to pass the fully qualified identifier of the resource for which the role assignment needs to be created
	RoleAssignmentScope = "roleassignmentscope"
)

// NewAzureRoleAssignmentHandler initializes a new handler for resources of kind RoleAssignment
func NewAzureRoleAssignmentHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureRoleAssignmentHandler{arm: arm}
}

type azureRoleAssignmentHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureRoleAssignmentHandler) Put(ctx context.Context, resource *outputresource.OutputResource) error {
	logger := radlogger.GetLogger(ctx)
	properties, err := handler.GetResourceNativeIdentityKeyProperties(ctx, *resource)
	if err != nil {
		return err
	}

	roleName := properties[RoleNameKey]
	scope := properties[RoleAssignmentScope]

	// Get dependencies
	managedIdentityProperties := map[string]string{}
	if prop, ok := properties[outputresource.LocalIDUserAssignedManagedIdentity]; ok {
		managedIdentityProperties[outputresource.LocalIDUserAssignedManagedIdentity] = prop
	}

	if len(managedIdentityProperties) == 0 {
		return errors.New("missing dependency: a user assigned identity is required to create role assignment")
	}

	// Assign Key Vault Secrets User role to grant managed identity read-only access to the keyvault for secrets.
	// Assign Key Vault Crypto User role to grant managed identity permissions to perform operations using encryption keys.
	roleAssignment, err := roleassignment.Create(ctx, handler.arm.Auth, handler.arm.SubscriptionID, managedIdentityProperties[UserAssignedIdentityPrincipalIDKey], scope, roleName)
	if err != nil {
		return fmt.Errorf(
			"failed to assign '%s' role to the managed identity '%s' within resource '%s' scope : %w",
			roleName,
			managedIdentityProperties[UserAssignedIdentityIDKey],
			scope,
			err)
	}
	logger.WithValues(radlogger.LogFieldLocalID, outputresource.LocalIDRoleAssignmentKVKeys).Info(fmt.Sprintf("Created %s role assignment for %s to access %s", roleName, managedIdentityProperties[UserAssignedIdentityIDKey], scope))

	resource.Identity = resourcemodel.NewARMIdentity(&resource.ResourceType, *roleAssignment.ID, clients.GetAPIVersionFromUserAgent(authorization.UserAgent()))
	return nil
}

func (handler *azureRoleAssignmentHandler) GetResourceIdentity(ctx context.Context, resource outputresource.OutputResource) (resourcemodel.ResourceIdentity, error) {
	properties, err := handler.GetResourceNativeIdentityKeyProperties(ctx, resource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, err
	}
	roleName := properties[RoleNameKey]
	scope := properties[RoleAssignmentScope]
	roleDefinitionID, err := roleassignment.GetRoleDefinitionID(ctx, handler.arm.Auth, handler.arm.SubscriptionID, scope, roleName)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, err
	}
	identity := resourcemodel.NewARMIdentity(&resource.ResourceType, roleDefinitionID, clients.GetAPIVersionFromUserAgent(authorization.UserAgent()))

	return identity, nil
}

func (handler *azureRoleAssignmentHandler) GetResourceNativeIdentityKeyProperties(ctx context.Context, resource outputresource.OutputResource) (map[string]string, error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
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

func (handler *azureRoleAssignmentHandler) Delete(ctx context.Context, resource outputresource.OutputResource) error {
	return nil
}
