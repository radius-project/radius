// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package roleassignment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/authorization/mgmt/authorization"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/gofrs/uuid"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/radlogger"
)

// Create assigns the specified role name to the Identity over the specified scope
// principalID - The principal ID assigned to the role. This maps to the ID inside the Active Directory. It can point to a user, service principal, or security group.
// scope - fully qualified identifier of the scope of the role assignment to create. Example: '/subscriptions/{subscription-id}/',
// '/subscriptions/{subscription-id}/resourceGroups/{resource-group-name}/providers/{resource-provider}/{resource-type}/{resource-name}'
// roleNameOrID - Name of the role ('Reader') or definition id ('acdd72a7-3385-48ef-bd42-f606fba81ae7') for the role to be assigned.
func Create(ctx context.Context, auth autorest.Authorizer, subscriptionID, principalID, scope, roleNameOrID string) (*authorization.RoleAssignment, error) {
	logger := radlogger.GetLogger(ctx)

	roleDefinitionID, err := GetRoleDefinitionID(ctx, auth, subscriptionID, scope, roleNameOrID)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment for role '%s': %w", roleNameOrID, err)
	}

	// Check if role assignment already exists for the managed identity
	roleAssignmentClient := clients.NewRoleAssignmentsClient(subscriptionID, auth)
	existingRoleAssignments, err := roleAssignmentClient.List(ctx, fmt.Sprintf("principalID eq '%s'", principalID), "")
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment for role '%s': %w", roleNameOrID, err)
	}
	for _, roleAssignment := range existingRoleAssignments.Values() {
		if roleDefinitionID == *roleAssignment.RoleAssignmentPropertiesWithScope.RoleDefinitionID &&
			scope == *roleAssignment.RoleAssignmentPropertiesWithScope.Scope {
			// The required role assignment already exists
			return &roleAssignment, nil
		}
	}

	// Generate a new role assignment name
	raName, err := uuid.NewV4()
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment for role '%s': %w", roleNameOrID, err)
	}

	// Retry to wait for the managed identity to propagate
	MaxRetries := 100
	// var ra authorization.RoleAssignment
	for i := 0; i < MaxRetries; i++ {
		roleAssignment, err := roleAssignmentClient.Create(
			ctx,
			scope,
			raName.String(),
			authorization.RoleAssignmentCreateParameters{
				RoleAssignmentProperties: &authorization.RoleAssignmentProperties{
					PrincipalID:      &principalID,
					RoleDefinitionID: to.StringPtr(roleDefinitionID),
				},
			})

		if err == nil {
			return &roleAssignment, nil
		}

		// Check the error and determine if it is ignorable/retryable
		detailedError, ok := clients.ExtractDetailedError(err)
		if !ok {
			return nil, err
		}

		// Sometimes, the managed identity takes a while to propagate and the role assignment creation fails with status code = 400
		// For other reasons, fail.
		if detailedError.StatusCode != 400 {
			return nil, fmt.Errorf("failed to create role assignment for role '%s' with error: %v, status code: %v", roleNameOrID, detailedError.Message, detailedError.StatusCode)
		}

		logger.Info(fmt.Sprintf("Failed to create role assignment for role '%s': %v. Retrying: attempt %d ...", roleNameOrID, err, i))
		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("failed to create role assignment for role '%s': %w", roleNameOrID, err)
}

// Returns roleDefinitionID: fully qualified identifier of role definition, example: "/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c"
func GetRoleDefinitionID(ctx context.Context, auth autorest.Authorizer, subscriptionID, scope, roleNameOrID string) (roleDefinitionID string, err error) {
	if strings.HasPrefix(roleNameOrID, "/subscriptions/") {
		roleDefinitionID = roleNameOrID
		return
	}

	roleDefinitionClient := clients.NewRoleDefinitionsClient(subscriptionID, auth)
	roleFilter := fmt.Sprintf("roleName eq '%s'", roleNameOrID)
	roleList, err := roleDefinitionClient.List(ctx, scope, roleFilter)
	if err != nil {
		return "", err
	}

	if len(roleList.Values()) == 0 {
		// Check if the passed value is a role definition id instead of role name. For example - id for role name "Contributor" is "b24988ac-6180-42a0-ab88-20f7382dd24c"
		// https://docs.microsoft.com/en-us/azure/role-based-access-control/built-in-roles
		roleDefinition, err := roleDefinitionClient.Get(ctx, scope, roleNameOrID)
		if err != nil {
			return "", err
		} else if roleDefinition == (authorization.RoleDefinition{}) {
			return "", fmt.Errorf("no role definition was found for the provided role %s", roleNameOrID)
		} else {
			roleDefinitionID = *roleDefinition.ID
		}
	} else {
		roleDefinitionID = *roleList.Values()[0].ID
	}

	return
}
