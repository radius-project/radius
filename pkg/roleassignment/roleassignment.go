// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package roleassignment

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/authorization/mgmt/authorization"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/gofrs/uuid"
)

// Create assigns the specified role name to the Identity over the specified scope
func Create(ctx context.Context, auth autorest.Authorizer, subscriptionID string, resourceGroup, principalID, scope, roleName string) (*authorization.RoleAssignment, error) {
	rdc := authorization.NewRoleDefinitionsClient(subscriptionID)
	rdc.Authorizer = auth

	roleFilter := fmt.Sprintf("roleName eq '%s'", roleName)
	roleList, err := rdc.List(ctx, scope, roleFilter)
	if err != nil || !roleList.NotDone() {
		return nil, fmt.Errorf("failed to create role assignment for user assigned managed identity: %w", err)
	}

	rac := authorization.NewRoleAssignmentsClient(subscriptionID)
	rac.Authorizer = auth
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
		detailed, ok := util.ExtractDetailedError(err)
		if !ok {
			return nil, err
		}
		// StatusCode = 409 indicates that the role assignment already exists. Ignore that error
		if detailed.StatusCode == 409 {
			return nil, nil
		}

		// Sometimes, the managed identity takes a while to propagate and the role assignment creation fails with status code = 400
		// For other reasons, fail.
		if detailed.StatusCode != 400 {
			return nil, fmt.Errorf("failed to create role assignment with error: %v, statuscode: %v", detailed.Message, detailed.StatusCode)
		}

		log.Printf("Failed to create role assignment. Retrying: %d attempt ...", i)
		time.Sleep(5 * time.Second)
	}

	return nil, nil
}
