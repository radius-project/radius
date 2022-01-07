// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package roleassignment

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/authorization/mgmt/authorization"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/gofrs/uuid"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/radlogger"
)

// Create assigns the specified role name to the Identity over the specified scope
func Create(ctx context.Context, auth autorest.Authorizer, subscriptionID string, resourceGroup, principalID, scope, roleName string) (*authorization.RoleAssignment, error) {
	logger := radlogger.GetLogger(ctx)
	rdc := clients.NewRoleDefinitionsClient(subscriptionID, auth)

	roleFilter := fmt.Sprintf("roleName eq '%s'", roleName)
	roleList, err := rdc.List(ctx, scope, roleFilter)
	if err != nil || !roleList.NotDone() {
		return nil, fmt.Errorf("failed to create role assignment for user assigned managed identity: %w", err)
	}

	rac := clients.NewRoleAssignmentsClient(subscriptionID, auth)

	// Check if role assignment already exists for the managed identity
	existing, err := rac.List(ctx, fmt.Sprintf("principalID eq '%s'", principalID), "")
	if err != nil {
		return nil, fmt.Errorf("failed to list role assignments for user assigned managed identity: %w", err)
	}
	for _, r := range existing.Values() {
		if *roleList.Values()[0].ID == *r.RoleAssignmentPropertiesWithScope.RoleDefinitionID &&
			scope == *r.RoleAssignmentPropertiesWithScope.Scope {
			// The required role assignment already exists
			return &r, nil
		}
	}

	// Generate a new role assignment name
	raName, _ := uuid.NewV4()

	MaxRetries := 100
	var ra authorization.RoleAssignment
	for i := 0; i <= MaxRetries; i++ {

		// Retry to wait for the managed identity to propagate
		if i >= MaxRetries {
			return nil, fmt.Errorf("failed to create role assignment for user assigned managed identity after %d retries: %w", i, err)
		}

		ra, err = rac.Create(
			ctx,
			scope,
			raName.String(),
			authorization.RoleAssignmentCreateParameters{
				RoleAssignmentProperties: &authorization.RoleAssignmentProperties{
					PrincipalID:      &principalID,
					RoleDefinitionID: to.StringPtr(*roleList.Values()[0].ID),
				},
			})

		if err == nil {
			return &ra, nil
		}

		// Check the error and determine if it is ignorable/retryable
		detailed, ok := clients.ExtractDetailedError(err)
		if !ok {
			return nil, err
		}

		// Sometimes, the managed identity takes a while to propagate and the role assignment creation fails with status code = 400
		// For other reasons, fail.
		if detailed.StatusCode != 400 {
			return nil, fmt.Errorf("failed to create role assignment with error: %v, statuscode: %v", detailed.Message, detailed.StatusCode)
		}

		logger.Info(fmt.Sprintf("Failed to create role assignment %v. Retrying: %d attempt ...", err, i))
		time.Sleep(5 * time.Second)
	}

	return nil, nil
}
