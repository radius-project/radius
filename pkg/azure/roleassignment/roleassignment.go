/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package roleassignment

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	armauthorization "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/to"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// Create assigns the specified role name to the Identity over the specified scope
// principalID - The principal ID assigned to the role. This maps to the ID inside the Active Directory. It can point to a user, service principal, or security group.
// scope - fully qualified identifier of the scope of the role assignment to create. Example: '/subscriptions/{subscription-id}/',
// '/subscriptions/{subscription-id}/resourceGroups/{resource-group-name}/providers/{resource-provider}/{resource-type}/{resource-name}'
// roleNameOrID - Name of the role ('Reader') or definition id ('acdd72a7-3385-48ef-bd42-f606fba81ae7') for the role to be assigned.
func Create(ctx context.Context, armConfig *armauth.ArmConfig, subscriptionID, principalID, scope, roleNameOrID string) (*armauthorization.RoleAssignment, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	roleDefinitionID, err := GetRoleDefinitionID(ctx, armConfig, subscriptionID, scope, roleNameOrID)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment for role '%s': %w", roleNameOrID, err)
	}

	// Check if role assignment already exists for the managed identity
	client, err := clientv2.NewRoleAssignmentsClient(subscriptionID, &armConfig.ClientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignments client: %w", err)
	}

	requestFilter := url.QueryEscape("principalId eq '" + principalID + "'")
	pager := client.NewListForScopePager(scope, &armauthorization.RoleAssignmentsClientListForScopeOptions{
		Filter: &requestFilter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list role assignments: %w", err)
	}

	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, ra := range nextPage.RoleAssignmentListResult.Value {
			if roleDefinitionID == *ra.Properties.RoleDefinitionID && scope == *ra.Properties.Scope {
				return ra, nil
			}
		}
	}

	// Generate a new role assignment name
	raName := uuid.New()

	// Retry to wait for the managed identity to propagate
	MaxRetries := 100
	// var ra authorization.RoleAssignment
	for i := 0; i < MaxRetries; i++ {
		resp, err := client.Create(
			ctx,
			scope,
			raName.String(),
			armauthorization.RoleAssignmentCreateParameters{
				Properties: &armauthorization.RoleAssignmentProperties{
					PrincipalID:      &principalID,
					RoleDefinitionID: to.Ptr(roleDefinitionID),
					PrincipalType:    to.Ptr(armauthorization.PrincipalTypeServicePrincipal),
				},
			},
			&armauthorization.RoleAssignmentsClientCreateOptions{})

		if err == nil {
			return &resp.RoleAssignment, nil
		}

		// Check the error and determine if it is ignorable/retryable
		respErr, ok := clientv2.ExtractResponseError(err)
		if !ok {
			return nil, err
		}

		// Sometimes, the managed identity takes a while to propagate and the role assignment creation fails with status code = 400
		// For other reasons, fail.
		if respErr.StatusCode != http.StatusBadRequest {
			return nil, fmt.Errorf("failed to create role assignment for role '%s' with error: %v, status code: %v",
				roleNameOrID, respErr.Error(), respErr.StatusCode)
		}

		logger.Info(fmt.Sprintf("Failed to create role assignment for role '%s': %v. Retrying: attempt %d ...", roleNameOrID, err, i))
		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("failed to create role assignment for role '%s': %w", roleNameOrID, err)
}

// Delete deletes the specified role name over the specified scope.
func Delete(ctx context.Context, armConfig *armauth.ArmConfig, roleID string) error {
	rID, err := resources.Parse(roleID)
	if err != nil {
		return err
	}

	subscriptionID := rID.FindScope(resources.SubscriptionsSegment)
	if subscriptionID == "" {
		return fmt.Errorf("invalid role id: %s", roleID)
	}

	client, err := clientv2.NewRoleAssignmentsClient(subscriptionID, &armConfig.ClientOptions)
	if err != nil {
		return fmt.Errorf("failed to create role assignments client: %w", err)
	}

	// Deleting nonexisting role returns 204 so we do not need to check the existence.
	_, err = client.DeleteByID(ctx, roleID, &armauthorization.RoleAssignmentsClientDeleteByIDOptions{})
	if err != nil {
		return err
	}

	return nil
}

// Returns roleDefinitionID: fully qualified identifier of role definition, example: "/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c"
func GetRoleDefinitionID(ctx context.Context, armConfig *armauth.ArmConfig, subscriptionID, scope, roleNameOrID string) (roleDefinitionID string, err error) {
	if strings.HasPrefix(roleNameOrID, "/subscriptions/") {
		roleDefinitionID = roleNameOrID
		return
	}

	client, err := clientv2.NewRoleDefinitionsClient(&armConfig.ClientOptions)
	if err != nil {
		return "", fmt.Errorf("failed to create role definitions client: %w", err)
	}

	requestFilter := fmt.Sprintf("roleName eq '%s'", roleNameOrID)
	pager := client.NewListPager(scope, &armauthorization.RoleDefinitionsClientListOptions{
		Filter: &requestFilter,
	})

	rds := []*armauthorization.RoleDefinition{}
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}

		rds = append(rds, nextPage.RoleDefinitionListResult.Value...)
	}

	if len(rds) == 0 {
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
		roleDefinitionID = *rds[0].ID
	}

	return
}
