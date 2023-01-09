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
	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/roleassignment"
	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
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

// Put assigns the selected roles to the identity.
func (handler *azureRoleAssignmentHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := logr.FromContextOrDiscard(ctx)

	properties, ok := options.Resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	roleName := properties[RoleNameKey]
	scope := properties[RoleAssignmentScope]

	// Get dependency
	identityProp, ok := options.DependencyProperties[outputresource.LocalIDUserAssignedManagedIdentity]
	if !ok {
		return nil, errors.New("missing dependency: a user assigned identity is required to create role assignment")
	}

	principalID, ok := identityProp[UserAssignedIdentityPrincipalIDKey]
	if !ok {
		return nil, errors.New("missing dependency: Principal ID was not populated in the previous resource handler")
	}

	// Scope may be a resource ID or an azure scope. We don't really need to know which so we're using the generic 'Parse' function.
	parsedScope, err := resources.ParseResource(scope)
	if err != nil {
		return nil, err
	}

	// Assign Key Vault Secrets User role to grant managed identity read-only access to the keyvault for secrets.
	// Assign Key Vault Crypto User role to grant managed identity permissions to perform operations using encryption keys.
	roleAssignment, err := roleassignment.Create(ctx, handler.arm.Auth, parsedScope.FindScope(resources.SubscriptionsSegment), principalID, scope, roleName)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to assign '%s' role to the managed identity '%s' within resource '%s' scope : %w",
			roleName, principalID, scope, err)
	}
	logger.WithValues(logging.LogFieldLocalID, outputresource.LocalIDRoleAssignmentKVKeys).Info(fmt.Sprintf("Created %s role assignment for %s to access %s", roleName, principalID, scope))

	options.Resource.Identity = resourcemodel.NewARMIdentity(&options.Resource.ResourceType, *roleAssignment.ID, clients.GetAPIVersionFromUserAgent(authorization.UserAgent()))
	return properties, nil
}

// Delete deletes the role from the resource.
func (handler *azureRoleAssignmentHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	roleID, _, err := options.Resource.Identity.RequireARM()
	if err != nil {
		return err
	}
	return roleassignment.Delete(ctx, handler.arm.Auth, roleID)
}
