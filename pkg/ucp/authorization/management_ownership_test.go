// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

func TestManagementBootstrapPromotionEveryMode(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce} {
		for _, typ := range []api.PrincipalType{api.PrincipalTypeUser, api.PrincipalTypeGroup} {
			t.Run(string(mode)+"/"+string(typ), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					policy := initialPolicy()
					policy.Settings.Properties.Mode = &mode
					for _, assignment := range policy.Assignments {
						assignment.Properties.ManagedBy.Kind = to.Ptr(api.AssignmentManagementKindBootstrap)
					}
					assignment := policy.Assignments[0]
					if typ == api.PrincipalTypeGroup {
						assignment = policy.Assignments[1]
					}
					_, runtime := managementRuntime(t, inmemory.NewClient(), policy)
					admin := policyActor().Identity
					handler := managementHandler(runtime, "")
					body, err := json.Marshal(map[string]any{"properties": map[string]any{
						"roleDefinitionId": assignment.Properties.RoleDefinitionID,
						"principal":        assignment.Properties.Principal,
					}})
					require.NoError(t, err)
					// Deletion and immutable-tuple changes cannot turn bootstrap
					// authority into a different permanent grant.
					response := managementRequest(t, handler, &admin, "DELETE", *assignment.ID+managementVersion+"&takeOwnership=true", "")
					require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), "ManagedByConflict")
					response = managementRequest(t, handler, &admin, "PUT", *assignment.ID+managementVersion,
						strings.Replace(string(body), "radius-administrator", "reader", 1))
					require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), "RoleAssignmentImmutable")
					require.Equal(t, int64(0), runtime.State().Generation)

					for range 2 {
						response = managementRequest(t, handler, &admin, "PUT", *assignment.ID+managementVersion, string(body))
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
						var promoted api.RoleAssignmentResource
						require.NoError(t, json.Unmarshal(response.Body.Bytes(), &promoted))
						require.Equal(t, assignment.ID, promoted.ID)
						require.Equal(t, assignment.Properties.Principal, promoted.Properties.Principal)
						require.Equal(t, assignment.Properties.RoleDefinitionID, promoted.Properties.RoleDefinitionID)
						require.Equal(t, assignment.Properties.CreatedBy, promoted.Properties.CreatedBy)
						require.Equal(t, assignment.Properties.Scope, promoted.Properties.Scope)
						require.Equal(t, api.AssignmentManagementKindImperative, *promoted.Properties.ManagedBy.Kind)
					}
					require.Equal(t, int64(1), runtime.State().Generation, "promotion is applied once")
					response = managementRequest(t, handler, &admin, "GET", historyPath+"/1"+managementVersion, "")
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					var event api.AccessChangeResource
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &event))
					require.True(t, *event.Properties.HighImpact)
					require.Equal(t, api.PolicyChangeOperationUpdate, *event.Properties.PolicyChange.Operation)
					require.Equal(t, api.AssignmentManagementKindBootstrap, *event.Properties.PolicyChange.Before.RoleAssignment.ManagedBy.Kind)
					require.Equal(t, api.AssignmentManagementKindImperative, *event.Properties.PolicyChange.After.RoleAssignment.ManagedBy.Kind)
					require.Equal(t, assignment.Properties.CreatedBy, event.Properties.PolicyChange.After.RoleAssignment.CreatedBy)
					require.Equal(t, admin.Principal, *event.Properties.Principal)
					response = managementRequest(t, handler, &admin, "DELETE", *assignment.ID+managementVersion, "")
					require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), "LastAdministratorProtected")
					require.Equal(t, int64(1), runtime.State().Generation)
				})
			})
		}
	}
}

func TestManagementGitOpsOwnershipEveryMode(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				policy := initialPolicy()
				policy.Settings.Properties.Mode = &mode
				principal := testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob")
				gitops := testAssignment("gitops", builtInID("reader"), teamA, principal)
				gitops.Properties.CreatedBy = to.Ptr("verified-controller")
				gitops.Properties.ManagedBy = &api.AssignmentManagement{
					Kind: to.Ptr(api.AssignmentManagementKindGitOps), Source: to.Ptr("https://example.test/team-a"),
					Revision: to.Ptr("verified-commit"), Path: to.Ptr("access.bicep"),
				}
				policy.Assignments = append(policy.Assignments, gitops)
				// Delete permission alone must not permit an ownership override.
				roleID := teamA + "/providers/Radius.Core/roleDefinitions/deleter"
				policy.CustomRoles = append(policy.CustomRoles, customRole(roleID, "Radius.Core/roleAssignments/delete"))
				deleter := testAssignment("deleter", roleID, teamA, principal)
				deleter.Properties.CreatedBy = to.Ptr("trusted-startup")
				deleter.Properties.ManagedBy = &api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)}
				policy.Assignments = append(policy.Assignments, deleter)
				_, runtime := managementRuntime(t, inmemory.NewClient(), policy)
				handler := managementHandler(runtime, "")
				admin := policyActor().Identity
				bob := Identity{Principal: principal}
				for _, test := range []struct {
					identity Identity
					method   string
					query    string
					body     string
				}{
					{admin, "PUT", "", assignmentBody(builtInID("reader"), "bob")},
					{admin, "DELETE", "", ""},
					{bob, "DELETE", "&takeOwnership=true", ""},
				} {
					request := httptest.NewRequest(test.method, *gitops.ID+managementVersion+test.query, strings.NewReader(test.body))
					request = request.WithContext(context.WithValue(request.Context(), identityContextKey{}, test.identity))
					// Even a Radius admin cannot assert a controller's provenance.
					request.Header.Set("X-Radius-Managed-By", "GitOps")
					request.Header.Set("X-Radius-Git-Source", *gitops.Properties.ManagedBy.Source)
					request.Header.Set("X-Radius-Git-Commit", *gitops.Properties.ManagedBy.Revision)
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)
					require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), "ManagedByConflict")
				}
				require.Equal(t, int64(0), runtime.State().Generation)
				response := managementRequest(t, handler, &admin, "DELETE", *gitops.ID+managementVersion+"&takeOwnership=true", "")
				require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
				response = managementRequest(t, handler, &admin, "GET", historyPath+"/1"+managementVersion, "")
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var event api.AccessChangeResource
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &event))
				require.True(t, *event.Properties.HighImpact)
				require.Equal(t, api.AssignmentManagementKindImperative, *event.Properties.PolicyChange.ManagedBy.Kind)
				require.Equal(t, gitops.Properties.ManagedBy, event.Properties.PolicyChange.Before.RoleAssignment.ManagedBy)
				require.Nil(t, event.Properties.PolicyChange.After)
				require.Equal(t, admin.Principal, *event.Properties.Principal)
			})
		})
	}
}
