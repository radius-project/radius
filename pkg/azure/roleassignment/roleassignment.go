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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/google/uuid"

	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// Create assigns the specified role name to the Identity over the specified scope
// principalID - The principal ID assigned to the role. This maps to the ID inside the Active Directory. It can point to a user, service principal, or security group.
// scope - fully qualified identifier of the scope of the role assignment to create. Example: '/subscriptions/{subscription-id}/',
// '/subscriptions/{subscription-id}/resourceGroups/{resource-group-name}/providers/{resource-provider}/{resource-type}/{resource-name}'
// roleNameOrID - Name of the role ('Reader') or definition id ('acdd72a7-3385-48ef-bd42-f606fba81ae7') for the role to be assigned.
func Create(ctx context.Context, subscriptionID, principalID, scope, roleNameOrID string) (*armauthorization.RoleAssignment, error) {
	logger := radlogger.GetLogger(ctx)

	// FIXME: What should the credential be?
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment credential for role '%s': %w", roleNameOrID, err)
	}

	roleDefinitionID, err := GetRoleDefinitionID(ctx, cred, subscriptionID, scope, roleNameOrID)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment for role '%s': %w", roleNameOrID, err)
	}

	client, err := armauthorization.NewRoleAssignmentsClient(subscriptionID, cred, &arm.ClientOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment client for role '%s': %w", roleNameOrID, err)
	}

	pager := client.NewListPager(&armauthorization.RoleAssignmentsClientListOptions{
		Filter: to.Ptr(fmt.Sprintf("principalID eq '%s'", principalID)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list role assignments for principalID '%s': %w", principalID, err)
	}

	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return &armauthorization.RoleAssignment{}, err
		}
		roleAssignments := nextPage.Value
		for _, roleAssignment := range roleAssignments {
			if roleDefinitionID == *roleAssignment.ID && scope == *roleAssignment.Properties.Scope {
				return roleAssignment, nil
			}
		}
	}

	// Generate a new role assignment name
	raName := uuid.New()

	// Retry to wait for the managed identity to propagate
	maxRetries := 100
	for i := 0; i < maxRetries; i++ {
		resp, err := client.Create(
			ctx,
			scope,
			raName.String(),
			armauthorization.RoleAssignmentCreateParameters{
				Properties: &armauthorization.RoleAssignmentProperties{
					PrincipalID:      &principalID,
					RoleDefinitionID: to.Ptr(roleDefinitionID),
				},
			},
			&armauthorization.RoleAssignmentsClientCreateOptions{})

		if err == nil {
			return &resp.RoleAssignment, nil
		}

		// Check the error and determine if it is ignorable/retryable
		detailedError, ok := clientv2.ExtractDetailedError(err)
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
func Delete(ctx context.Context, cred azcore.TokenCredential, roleID string) error {
	rID, err := resources.Parse(roleID)
	if err != nil {
		return err
	}

	subscriptionID := rID.FindScope(resources.SubscriptionsSegment)
	if subscriptionID == "" {
		return fmt.Errorf("invalid role id: %s", roleID)
	}

	client, err := armauthorization.NewRoleAssignmentsClient(subscriptionID, cred, &arm.ClientOptions{})
	if err != nil {
		return fmt.Errorf("failed to create role assignment client for role '%s': %w", roleID, err)
	}

	// Deleting nonexisting role returns 204 so we do not need to check the existence.
	_, err = client.DeleteByID(ctx, roleID, &armauthorization.RoleAssignmentsClientDeleteByIDOptions{})
	if err == nil {
		return nil
	}

	// Extract the detailedError from error.
	detailedError, ok := clientv2.ExtractDetailedError(err)
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
func GetRoleDefinitionID(ctx context.Context, cred azcore.TokenCredential, subscriptionID, scope, roleNameOrID string) (string, error) {
	roleDefinitionID := ""

	if strings.HasPrefix(roleNameOrID, "/subscriptions/") {
		roleDefinitionID = roleNameOrID
		return roleDefinitionID, nil
	}

	client, err := armauthorization.NewRoleDefinitionsClient(cred, &arm.ClientOptions{})
	if err != nil {
		return roleDefinitionID, err
	}

	roleFilter := fmt.Sprintf("roleName eq '%s'", roleNameOrID)
	pager := client.NewListPager(scope, &armauthorization.RoleDefinitionsClientListOptions{
		Filter: &roleFilter,
	})

	var roleDefinitions []*armauthorization.RoleDefinition
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}
		rds := nextPage.Value
		for _, rd := range rds {
			roleDefinitions = append(roleDefinitions, rd)
		}
	}

	if len(roleDefinitions) == 0 {
		// Check if the passed value is a role definition id instead of role name. For example - id for role name "Contributor" is "b24988ac-6180-42a0-ab88-20f7382dd24c"
		// https://docs.microsoft.com/en-us/azure/role-based-access-control/built-in-roles
		resp, err := client.Get(ctx, scope, roleNameOrID, &armauthorization.RoleDefinitionsClientGetOptions{})
		if err != nil {
			return "", err
		} else if resp.RoleDefinition == (armauthorization.RoleDefinition{}) {
			return "", fmt.Errorf("no role definition was found for the provided role %s", roleNameOrID)
		} else {
			roleDefinitionID = *resp.RoleDefinition.ID
		}
	} else {
		roleDefinitionID = *roleDefinitions[0].ID
	}

	return roleDefinitionID, nil
}
