/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/azure/roleassignment"
	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
	logger := ucplog.FromContextOrDiscard(ctx)

	properties, ok := options.Resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	roleName := properties[RoleNameKey]
	scope := properties[RoleAssignmentScope]

	// Get dependency
	identityProp, ok := options.DependencyProperties[rpv1.LocalIDUserAssignedManagedIdentity]
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
	roleAssignment, err := roleassignment.Create(ctx, handler.arm, parsedScope.FindScope(resources.SubscriptionsSegment), principalID, scope, roleName)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to assign '%s' role to the managed identity '%s' within resource '%s' scope : %w",
			roleName, principalID, scope, err)
	}
	logger.Info(fmt.Sprintf("Created %s role assignment for %s to access %s", roleName, principalID, scope), logging.LogFieldLocalID, rpv1.LocalIDRoleAssignmentKVKeys)

	options.Resource.Identity = resourcemodel.NewARMIdentity(&options.Resource.ResourceType, *roleAssignment.ID, clientv2.RoleAssignmentClientAPIVersion)
	return properties, nil
}

// Delete deletes the role from the resource.
func (handler *azureRoleAssignmentHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	roleID, _, err := options.Resource.Identity.RequireARM()
	if err != nil {
		return err
	}
	return roleassignment.Delete(ctx, handler.arm, roleID)
}
