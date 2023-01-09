// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package roleassignment

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/authorization/mgmt/authorization"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest"
	"github.com/google/uuid"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// Create assigns the specified role name to the Identity over the specified scope
// principalID - The principal ID assigned to the role. This maps to the ID inside the Active Directory. It can point to a user, service principal, or security group.
// scope - fully qualified identifier of the scope of the role assignment to create. Example: '/subscriptions/{subscription-id}/',
// '/subscriptions/{subscription-id}/resourceGroups/{resource-group-name}/providers/{resource-provider}/{resource-type}/{resource-name}'
// roleNameOrID - Name of the role ('Reader') or definition id ('acdd72a7-3385-48ef-bd42-f606fba81ae7') for the role to be assigned.
func Create(ctx context.Context, auth autorest.Authorizer, subscriptionID, principalID, scope, roleNameOrID string) (*authorization.RoleAssignment, error) {
	logger := logr.FromContextOrDiscard(ctx)

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
	raName := uuid.New()

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
					RoleDefinitionID: to.Ptr(roleDefinitionID),
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
		if detailedError.StatusCode != http.StatusBadRequest {
			return nil, fmt.Errorf("failed to create role assignment for role '%s' with error: %v, status code: %v", roleNameOrID, detailedError.Message, detailedError.StatusCode)
		}

		logger.Info(fmt.Sprintf("Failed to create role assignment for role '%s': %v. Retrying: attempt %d ...", roleNameOrID, err, i))
		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("failed to create role assignment for role '%s': %w", roleNameOrID, err)
}

// Delete deletes the specified role name over the specified scope.
func Delete(ctx context.Context, auth autorest.Authorizer, roleID string) error {
	rID, err := resources.Parse(roleID)
	if err != nil {
		return err
	}

	subscriptionID := rID.FindScope(resources.SubscriptionsSegment)
	if subscriptionID == "" {
		return fmt.Errorf("invalid role id: %s", roleID)
	}

	roleAssignmentClient := clients.NewRoleAssignmentsClient(subscriptionID, auth)
	// Deleting nonexisting role returns 204 so we do not need to check the existence.
	_, err = roleAssignmentClient.DeleteByID(ctx, roleID, "")
	if err == nil {
		return nil
	}

	// Extract the detailedError from error.
	detailedError, ok := clients.ExtractDetailedError(err)
	if !ok {
		return err
	}

	// Ignore when it deletes role from non-existing or deleted resource.
	if detailedError.StatusCode == http.StatusNotFound {
		return nil
	}

	return fmt.Errorf("failed to delete role assignment for role '%s': %w", roleID, err)
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
