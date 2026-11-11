// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/clock"
)

const managementVersion = "?api-version=2025-08-01-preview"

func managementRequest(t *testing.T, handler http.Handler, identity *Identity, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if identity != nil {
		request = request.WithContext(context.WithValue(request.Context(), identityContextKey{}, *identity))
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func managementHandler(runtime *PolicyRuntime, pathBase string) http.Handler {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	return Management(runtime, pathBase)(ObserveRequests(runtime, pathBase)(fallback))
}

func managementRuntime(t *testing.T, db database.Client, policy Policy) (*PolicyStore, *PolicyRuntime) {
	t.Helper()
	store := newTestPolicyStore(t, db, false)
	initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
	require.NoError(t, err)
	require.NoError(t, initializer.Initialize(t.Context(), policy))
	runtime, _ := startPolicyRuntime(t, store)
	return store, runtime
}

func assignmentBody(role, subject string) string {
	return `{"properties":{"roleDefinitionId":"` + role + `","principal":{"type":"user","issuer":"kubernetes:cluster-a","subject":"` + subject + `"}}}`
}

func TestManagementLifecycleEveryMode(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				policy := initialPolicy()
				policy.Settings.Properties.Mode = &mode
				store, runtime := managementRuntime(t, inmemory.NewClient(), policy)
				const pathBase = "/apis/api.ucp.dev/v1alpha3"
				handler := managementHandler(runtime, pathBase)
				admin := policyActor().Identity
				// Group membership alone grants management access.
				admin.Principal.Subject = to.Ptr("group-member")
				roleID := strings.ToLower(teamA) + "/providers/radius.core/roledefinitions/operator"
				assignmentID := strings.ToLower(teamA) + "/providers/radius.core/roleassignments/operator"
				roleBody := `{"properties":{"displayName":"Operator","permissions":["Radius.Core/applications/read"]}}`
				for _, item := range []struct {
					path, body string
				}{
					{roleID, roleBody},
					{assignmentID, assignmentBody(roleID, "bob")},
				} {
					response := managementRequest(t, handler, &admin, http.MethodPut, pathBase+item.path+managementVersion, item.body)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					require.NotEmpty(t, response.Header().Get("X-Ms-Request-Id"))
					var resource map[string]any
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &resource))
					require.Equal(t, item.path, resource["id"])
					require.Equal(t, "operator", resource["name"])
					require.Contains(t, resource, "type")
					read := managementRequest(t, handler, &admin, http.MethodGet, pathBase+strings.ToUpper(item.path)+managementVersion, "")
					require.Equal(t, http.StatusOK, read.Code, read.Body.String())
					require.JSONEq(t, response.Body.String(), read.Body.String())
					// Repeated identical PUTs do not publish another generation.
					repeated := managementRequest(t, handler, &admin, http.MethodPut, pathBase+item.path+managementVersion, item.body)
					require.Equal(t, http.StatusOK, repeated.Code, repeated.Body.String())
				}
				require.Equal(t, int64(2), runtime.State().Generation)
				read := managementRequest(t, handler, &admin, http.MethodGet, pathBase+assignmentID+managementVersion, "")
				var assignment api.RoleAssignmentResource
				require.NoError(t, json.Unmarshal(read.Body.Bytes(), &assignment))
				require.Equal(t, strings.ToLower(teamA), *assignment.Properties.Scope)
				require.Equal(t, api.AssignmentManagementKindImperative, *assignment.Properties.ManagedBy.Kind)
				require.Equal(t, principalDisplay(admin.Principal), *assignment.Properties.CreatedBy)
				event, err := store.AccessChange(t.Context(), 2)
				require.NoError(t, err)
				require.Equal(t, admin.Principal, *event.Properties.Principal)
				require.Equal(t, mode, *event.Properties.Mode)
				require.NotEmpty(t, *event.Properties.RequestID)
				require.False(t, *event.Properties.Recovery)

				inUse := managementRequest(t, handler, &admin, http.MethodDelete, pathBase+roleID+managementVersion, "")
				require.Equal(t, http.StatusConflict, inUse.Code, inUse.Body.String())
				require.Contains(t, inUse.Body.String(), "RoleDefinitionInUse")
				for _, id := range []string{assignmentID, roleID} {
					for range 2 {
						deleted := managementRequest(t, handler, &admin, http.MethodDelete, pathBase+id+managementVersion, "")
						require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())
						require.Empty(t, deleted.Body.String())
					}
					missing := managementRequest(t, handler, &admin, http.MethodGet, pathBase+id+managementVersion, "")
					require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())
				}
				require.Equal(t, int64(4), runtime.State().Generation)
			})
		})
	}
}

func TestManagementAuthorizationEveryMode(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				policy := initialPolicy()
				policy.Settings.Properties.Mode = &mode
				_, runtime := managementRuntime(t, inmemory.NewClient(), policy)
				handler := managementHandler(runtime, "")
				bob := Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob")}
				for _, identity := range []*Identity{nil, &bob} {
					for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
						for _, path := range []string{
							"/providers/Radius.Core/roleAssignments/alpha",
							"/providers/Radius.Core/roleAssignments/missing",
							"/providers/Radius.Core/roleDefinitions/reader",
							"/providers/Radius.Core/roleDefinitions/missing",
						} {
							response := managementRequest(t, handler, identity, method, path+managementVersion, `{"properties":{}}`)
							status := http.StatusForbidden
							if identity == nil {
								status = http.StatusUnauthorized
							}
							require.Equal(t, status, response.Code, "%s %s: %s", method, path, response.Body.String())
							if identity != nil {
								require.Contains(t, response.Body.String(), `"type":"RadiusAuthorization"`)
								require.Contains(t, response.Body.String(), `"source":"Radius"`)
							}
						}
					}
				}
				require.Equal(t, int64(0), runtime.State().Generation)
			})
		})
	}
}

func TestManagementListsAndScopes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		policy := initialPolicy()
		policy.CustomRoles = []api.RoleDefinitionResource{
			customRole(teamA+"/providers/Radius.Core/roleDefinitions/local", "Radius.Core/applications/read"),
		}
		for _, scope := range []string{teamA, teamB, envA} {
			resource := testAssignment("reader", builtInID("reader"), scope, testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob"))
			resource.Properties.ManagedBy = &api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)}
			resource.Properties.CreatedBy = to.Ptr("trusted-startup")
			policy.Assignments = append(policy.Assignments, resource)
		}
		_, runtime := managementRuntime(t, inmemory.NewClient(), policy)
		handler := managementHandler(runtime, "")
		admin := policyActor().Identity
		for _, scenario := range []struct {
			path  string
			count int
		}{
			{"/providers/Radius.Core/roleDefinitions" + managementVersion, 9},
			{teamA + "/providers/Radius.Core/roleDefinitions" + managementVersion, 1},
			{teamB + "/providers/Radius.Core/roleDefinitions" + managementVersion, 0},
			{teamA + "/providers/Radius.Core/roleAssignments" + managementVersion, 1},
			{teamA + "/providers/Radius.Core/roleAssignments" + managementVersion + "&$filter=atScopeAndBelow()", 2},
			{"/providers/Radius.Core/roleAssignments" + managementVersion, 2},
			{"/providers/Radius.Core/roleAssignments" + managementVersion + "&$filter=atScopeAndBelow()", 5},
			{envA + "/providers/Radius.Core/roleAssignments" + managementVersion, 1},
		} {
			response := managementRequest(t, handler, &admin, http.MethodGet, scenario.path, "")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var list struct{ Value []json.RawMessage }
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &list))
			require.NotNil(t, list.Value)
			require.Len(t, list.Value, scenario.count, scenario.path)
		}
		for _, scope := range []string{"/", "/planes/radius/local", "/planes/azure/local", "/planes/aws/local", teamA} {
			prefix := strings.TrimSuffix(scope, "/")
			response := managementRequest(t, handler, &admin, http.MethodPut,
				prefix+"/providers/Radius.Core/roleDefinitions/created"+managementVersion,
				`{"properties":{"permissions":["Radius.Core/applications/read"]}}`)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		}
		for _, kind := range []string{"roleDefinitions", "roleAssignments"} {
			scope := teamA + "/providers/Example.Test/widgets/a"
			if kind == "roleDefinitions" {
				scope = envA
			}
			response := managementRequest(t, handler, &admin, http.MethodGet, scope+"/providers/Radius.Core/"+kind+managementVersion, "")
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		}
	})
}

func TestManagementInvalidInputsAndSafety(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, runtime := managementRuntime(t, inmemory.NewClient(), initialPolicy())
		handler := managementHandler(runtime, "")
		admin := policyActor().Identity
		const roleID = "/providers/Radius.Core/roleDefinitions/custom"
		const assignmentID = "/providers/Radius.Core/roleAssignments/custom"
		for _, scenario := range []struct {
			name, method, path, body, code string
			status                         int
		}{
			{"version", "GET", roleID, "", "InvalidApiVersionParameter", 400},
			{"duplicate version", "GET", roleID + managementVersion + "&api-version=2025-08-01-preview", "", "InvalidApiVersionParameter", 400},
			{"method", "PATCH", roleID + managementVersion, `{}`, "MethodNotAllowed", 405},
			{"collection put", "PUT", "/providers/Radius.Core/roleDefinitions" + managementVersion, `{}`, "MethodNotAllowed", 405},
			{"filter on role", "GET", roleID + managementVersion + "&$filter=atScopeAndBelow()", "", "InvalidRequestContent", 400},
			{"unknown filter", "GET", "/providers/Radius.Core/roleAssignments" + managementVersion + "&$filter=anything", "", "InvalidRequestContent", 400},
			{"duplicate filter", "GET", "/providers/Radius.Core/roleAssignments" + managementVersion + "&$filter=atScopeAndBelow()&$filter=atScopeAndBelow()", "", "InvalidRequestContent", 400},
			{"ownership on put", "PUT", assignmentID + managementVersion + "&takeOwnership=true", `{}`, "InvalidRequestContent", 400},
			{"bad ownership", "DELETE", assignmentID + managementVersion + "&takeOwnership=yes", "", "InvalidRequestContent", 400},
			{"null", "PUT", roleID + managementVersion, `null`, "InvalidRequestContent", 400},
			{"empty properties", "PUT", roleID + managementVersion, `{"properties":{}}`, "InvalidRequestContent", 400},
			{"null permission", "PUT", roleID + managementVersion, `{"properties":{"permissions":[null]}}`, "InvalidRequestContent", 400},
			{"unknown permission", "PUT", roleID + managementVersion, `{"properties":{"permissions":["Radius.Core/unknown/read"]}}`, "UnknownPermission", 400},
			{"role type spoof", "PUT", roleID + managementVersion, `{"properties":{"roleType":"BuiltIn","permissions":["Radius.Core/applications/read"]}}`, "InvalidRequestContent", 400},
			{"id spoof", "PUT", roleID + managementVersion, `{"id":"/other","properties":{"permissions":["Radius.Core/applications/read"]}}`, "InvalidRequestContent", 400},
			{"extra object", "PUT", roleID + managementVersion, `{"properties":{"permissions":["Radius.Core/applications/read"]}} {}`, "InvalidRequestContent", 400},
			{"builtin write", "PUT", builtInID("reader") + managementVersion, `{"properties":{"permissions":["Radius.Core/applications/read"]}}`, "BuiltInRoleImmutable", 409},
			{"builtin delete", "DELETE", builtInID("reader") + managementVersion, "", "BuiltInRoleImmutable", 409},
			{"missing assignment", "PUT", assignmentID + managementVersion, `{"properties":{}}`, "InvalidRequestContent", 400},
			{"unknown role", "PUT", assignmentID + managementVersion, assignmentBody(roleID, "bob"), "InvalidRequestContent", 400},
			{"invalid principal", "PUT", assignmentID + managementVersion, assignmentBody(builtInID("reader"), "system:anonymous"), "InvalidRequestContent", 400},
			{"ownership spoof", "PUT", assignmentID + managementVersion, `{"properties":{"roleDefinitionId":"/providers/Radius.Core/roleDefinitions/reader","principal":{"type":"user","issuer":"kubernetes:cluster-a","subject":"bob"},"managedBy":{"kind":"GitOps","source":"evil"}}}`, "InvalidRequestContent", 400},
			{"scope spoof", "PUT", assignmentID + managementVersion, `{"properties":{"scope":"/","roleDefinitionId":"/providers/Radius.Core/roleDefinitions/reader","principal":{"type":"user","issuer":"kubernetes:cluster-a","subject":"bob"}}}`, "InvalidRequestContent", 400},
			{"creator spoof", "PUT", assignmentID + managementVersion, `{"properties":{"createdBy":"administrator","roleDefinitionId":"/providers/Radius.Core/roleDefinitions/reader","principal":{"type":"user","issuer":"kubernetes:cluster-a","subject":"bob"}}}`, "InvalidRequestContent", 400},
			{"groups spoof", "PUT", assignmentID + managementVersion, `{"properties":{"roleDefinitionId":"/providers/Radius.Core/roleDefinitions/reader","principal":{"type":"user","issuer":"kubernetes:cluster-a","subject":"bob","groups":["admins"]}}}`, "InvalidRequestContent", 400},
			{"invalid role ID", "PUT", assignmentID + managementVersion, assignmentBody("/providers/Radius.Core/roleAssignments/other", "bob"), "InvalidRequestContent", 400},
			{"null scope", "PUT", roleID + managementVersion, `{"properties":{"permissions":["Radius.Core/applications/read"],"assignableScopes":[null]}}`, "InvalidRequestContent", 400},
			{"permission bound", "PUT", roleID + managementVersion, `{"properties":{"permissions":["` + strings.Repeat("x", 257) + `"]}}`, "InvalidRequestContent", 400},
			{"scope bound", "PUT", roleID + managementVersion, `{"properties":{"permissions":["Radius.Core/applications/read"],"assignableScopes":["/` + strings.Repeat("x", 2048) + `"]}}`, "InvalidRequestContent", 400},
			{"capacity", "PUT", roleID + managementVersion, `{"properties":{"description":"` + strings.Repeat("x", maxPolicyBytes) + `","permissions":["Radius.Core/applications/read"]}}`, "PolicyCapacityExceeded", 409},
			{"immutable assignment", "PUT", "/providers/Radius.Core/roleAssignments/alpha" + managementVersion, assignmentBody(builtInID("reader"), "mallory"), "RoleAssignmentImmutable", 409},
			{"body cap", "PUT", roleID + managementVersion, `{"properties":{"description":"` + strings.Repeat("x", 1<<20) + `","permissions":["Radius.Core/applications/read"]}}`, "InvalidRequestContent", 400},
			{"issuer length", "PUT", assignmentID + managementVersion, strings.Replace(assignmentBody(builtInID("reader"), "bob"), "kubernetes:cluster-a", strings.Repeat("x", 257), 1), "InvalidRequestContent", 400},
		} {
			response := managementRequest(t, handler, &admin, scenario.method, scenario.path, scenario.body)
			require.Equal(t, scenario.status, response.Code, "%s: %s", scenario.name, response.Body.String())
			require.Contains(t, response.Body.String(), scenario.code, scenario.name)
		}
		require.Equal(t, int64(0), runtime.State().Generation)
	})
}

func TestManagementEscalationAndScopeIsolation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		policy := initialPolicy()
		roleID := teamA + "/providers/Radius.Core/roleDefinitions/delegator"
		policy.CustomRoles = append(policy.CustomRoles, customRole(roleID,
			"Radius.Core/roleAssignments/write", "Radius.Core/roleAssignments/read", "Radius.Core/roleDefinitions/write"))
		bob := Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob")}
		grant := testAssignment("delegator", roleID, teamA, bob.Principal)
		grant.Properties.CreatedBy = to.Ptr("trusted-startup")
		grant.Properties.ManagedBy = &api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)}
		policy.Assignments = append(policy.Assignments, grant)
		_, runtime := managementRuntime(t, inmemory.NewClient(), policy)
		handler := managementHandler(runtime, "")
		for _, scenario := range []struct {
			path, body, code string
		}{
			{teamA + "/providers/Radius.Core/roleAssignments/admin", assignmentBody(builtInID("radius-administrator"), "mallory"), "PrivilegeEscalationDenied"},
			{teamA + "/providers/Radius.Core/roleDefinitions/escalation", `{"properties":{"permissions":["Radius.Core/applications/write"]}}`, "PrivilegeEscalationDenied"},
			{teamB + "/providers/Radius.Core/roleAssignments/reader", assignmentBody(builtInID("reader"), "mallory"), "AuthorizationFailed"},
			{teamA + "/providers/Radius.Core/roleDefinitions/elsewhere", `{"properties":{"permissions":["Radius.Core/roleAssignments/read"],"assignableScopes":["` + teamB + `"]}}`, "AuthorizationFailed"},
		} {
			response := managementRequest(t, handler, &bob, http.MethodPut, scenario.path+managementVersion, scenario.body)
			require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), scenario.code)
		}
		response := managementRequest(t, handler, &bob, http.MethodGet,
			teamA+"/providers/Radius.Core/roleAssignments"+managementVersion+"&$filter=atScopeAndBelow()", "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var list api.AuthorizationListRoleAssignmentResource
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &list))
		require.Len(t, list.Value, 1)
		require.Equal(t, strings.ToLower(*grant.ID), strings.ToLower(*list.Value[0].ID))
		for _, scope := range []string{"/", teamB} {
			response := managementRequest(t, handler, &bob, http.MethodGet,
				strings.TrimSuffix(scope, "/")+"/providers/Radius.Core/roleAssignments"+managementVersion, "")
			require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
		}
		require.Equal(t, int64(0), runtime.State().Generation)
	})
}

func TestManagementPermanentAdministratorProtection(t *testing.T) {
	for _, scenario := range []string{"bootstrap", "workload", "last group"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				policy := initialPolicy()
				target := *policy.Assignments[0].ID
				switch scenario {
				case "bootstrap":
					policy.Assignments[1].Properties.ManagedBy.Kind = to.Ptr(api.AssignmentManagementKindBootstrap)
				case "workload":
					policy.Assignments[1].Properties.Principal.Type = to.Ptr(api.PrincipalTypeWorkload)
				case "last group":
					policy.Assignments = policy.Assignments[1:]
					target = *policy.Assignments[0].ID
				}
				_, runtime := managementRuntime(t, inmemory.NewClient(), policy)
				admin := policyActor().Identity
				response := managementRequest(t, managementHandler(runtime, ""), &admin, http.MethodDelete, target+managementVersion, "")
				require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
				require.Contains(t, response.Body.String(), "LastAdministratorProtected")
				require.Equal(t, int64(0), runtime.State().Generation)
			})
		})
	}
}

func TestManagementOwnershipUsesSharedGuard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		policy := initialPolicy()
		grant := testAssignment("gitops", builtInID("reader"), teamA, testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-a", "readers"))
		grant.Properties.CreatedBy = to.Ptr("trusted-startup")
		grant.Properties.ManagedBy = &api.AssignmentManagement{
			Kind: to.Ptr(api.AssignmentManagementKindGitOps), Source: to.Ptr("https://example.test/owned"),
		}
		policy.Assignments = append(policy.Assignments, grant)
		store, runtime := managementRuntime(t, inmemory.NewClient(), policy)
		admin := policyActor().Identity
		handler := managementHandler(runtime, "")
		response := managementRequest(t, handler, &admin, http.MethodDelete, *grant.ID+managementVersion, "")
		require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "ManagedByConflict")
		response = managementRequest(t, handler, &admin, http.MethodDelete, *grant.ID+managementVersion+"&takeOwnership=true", "")
		require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
		event, err := store.AccessChange(t.Context(), 1)
		require.NoError(t, err)
		require.True(t, *event.Properties.HighImpact)
	})
}

func TestManagementCommitResponseSurvivesConcurrentRemoval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var armed atomic.Bool
		db := &policyHookClient{Client: inmemory.NewClient()}
		store, runtime := managementRuntime(t, db, initialPolicy())
		mutation := readerMutation()
		db.after = func(ctx context.Context, object *database.Object) error {
			if saveStage(object) == "finalize" && armed.CompareAndSwap(true, false) {
				_, err := store.Mutate(ctx, policyActor(), PolicyMutation{Target: mutation.Target, Delete: true})
				return err
			}
			return nil
		}
		armed.Store(true)
		admin := policyActor().Identity
		response := managementRequest(t, managementHandler(runtime, ""), &admin, http.MethodPut,
			mutation.Target+managementVersion, assignmentBody(builtInID("reader"), "bob"))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var resource api.RoleAssignmentResource
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &resource))
		require.Equal(t, strings.ToLower(mutation.Target), *resource.ID)
		require.Equal(t, int64(2), runtime.State().Generation)
		response = managementRequest(t, managementHandler(runtime, ""), &admin, http.MethodGet, mutation.Target+managementVersion, "")
		require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	})
}

func TestManagementPostCommitRefreshFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var armed, finalized atomic.Bool
		var reads atomic.Int32
		hook := &policyHookClient{Client: inmemory.NewClient(), after: func(_ context.Context, object *database.Object) error {
			if armed.Load() && saveStage(object) == "finalize" {
				finalized.Store(true)
			}
			return nil
		}}
		db := &runtimeDatabase{Client: hook}
		store, runtime := managementRuntime(t, db, initialPolicy())
		db.setGet(func(_ context.Context, id string) error {
			if finalized.Load() && id == accessVersionID && reads.Add(1) == 2 {
				return errors.New("private refresh failure")
			}
			return nil
		})
		armed.Store(true)
		admin := policyActor().Identity
		response := managementRequest(t, managementHandler(runtime, ""), &admin, http.MethodPut,
			readerMutation().Target+managementVersion, assignmentBody(builtInID("reader"), "bob"))
		require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), "private")
		require.Equal(t, int64(0), runtime.State().Generation)
		db.setGet(nil)
		snapshot, err := store.LoadPolicy(t.Context(), -1)
		require.NoError(t, err)
		require.Equal(t, int64(1), snapshot.Generation())
		_, err = store.AccessChange(t.Context(), 1)
		require.NoError(t, err)
	})
}

func TestManagementConcurrencyRetryLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var armed atomic.Bool
		db := &policyHookClient{Client: inmemory.NewClient(), before: func(_ context.Context, object *database.Object) error {
			if armed.Load() && saveStage(object) == "claim" {
				return &database.ErrConcurrency{}
			}
			return nil
		}}
		_, runtime := managementRuntime(t, db, initialPolicy())
		armed.Store(true)
		admin := policyActor().Identity
		response := managementRequest(t, managementHandler(runtime, ""), &admin, http.MethodPut,
			readerMutation().Target+managementVersion, assignmentBody(builtInID("reader"), "bob"))
		require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"code":"Conflict"`)
		require.Equal(t, int64(0), runtime.State().Generation)
	})
}

func TestManagementUnavailableAndFallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, runtime := managementRuntime(t, inmemory.NewClient(), initialPolicy())
		admin := policyActor().Identity
		const role = "/providers/Radius.Core/roleDefinitions/reader"
		for _, active := range []*PolicyRuntime{nil, runtime} {
			if active != nil {
				active.cache.mu.Lock()
				active.cache.asOf = active.cache.asOf.Add(-PolicyMaxAge)
				active.cache.mu.Unlock()
			}
			response := managementRequest(t, managementHandler(active, ""), &admin, http.MethodGet, role+managementVersion, "")
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
			require.Contains(t, response.Body.String(), "AuthorizationUnavailable")
			require.NotContains(t, response.Body.String(), "properties")
		}
		for _, path := range []string{
			"/providers/Radius.Core/authorizationSettings/default",
			"/providers/Radius.Core/accessChanges/1",
			"/providers/System.Authorization/accessVersions/default",
			"/providers/Radius.Core/roleAssignments/a/action",
			"/providers/Radius.Core/roleAssignments/a/",
			"/providers/Radius%252eCore/roleAssignments/a",
			"/providers//Radius.Core//roleAssignments/a",
		} {
			response := managementRequest(t, managementHandler(runtime, ""), &admin, http.MethodPut, path+managementVersion, `{}`)
			require.Equal(t, http.StatusServiceUnavailable, response.Code, path)
		}
		response := managementRequest(t, managementHandler(nil, ""), nil, http.MethodGet, "/ordinary", "")
		require.Equal(t, http.StatusTeapot, response.Code)
	})
}

func TestManagementConcurrentAdministratorRemoval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var armed atomic.Bool
		var claims atomic.Int32
		gate := make(chan struct{})
		db := &policyHookClient{Client: inmemory.NewClient(), before: func(_ context.Context, object *database.Object) error {
			if armed.Load() && saveStage(object) == "claim" {
				count := claims.Add(1)
				if count == 2 {
					close(gate)
				}
				if count <= 2 {
					<-gate
				}
			}
			return nil
		}}
		store, runtime := managementRuntime(t, db, initialPolicy())
		secondStore := newTestPolicyStore(t, db, false)
		second, _ := startPolicyRuntime(t, secondStore)
		armed.Store(true)
		results := make(chan *httptest.ResponseRecorder, 2)
		admin := policyActor().Identity
		for i, active := range []*PolicyRuntime{runtime, second} {
			name := []string{"alpha", "beta"}[i]
			go func() {
				results <- managementRequest(t, managementHandler(active, ""), &admin, http.MethodDelete,
					"/providers/Radius.Core/roleAssignments/"+name+managementVersion, "")
			}()
		}
		synctest.Wait()
		statuses := map[int]int{}
		for range 2 {
			response := <-results
			statuses[response.Code]++
			if response.Code == http.StatusConflict {
				require.Contains(t, response.Body.String(), "LastAdministratorProtected")
			}
		}
		require.Equal(t, map[int]int{http.StatusNoContent: 1, http.StatusConflict: 1}, statuses)
		snapshot, err := store.LoadPolicy(t.Context(), -1)
		require.NoError(t, err)
		require.Equal(t, int64(1), snapshot.Generation())
		policy, err := snapshot.Policy()
		require.NoError(t, err)
		require.Len(t, policy.Assignments, 1)
		require.Equal(t, 1, permanentAdministrators(policy))
		_, err = store.AccessChange(t.Context(), 1)
		require.NoError(t, err)
	})
}

func TestManagementStorageFailureIsUnavailable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := &runtimeDatabase{Client: inmemory.NewClient()}
		_, runtime := managementRuntime(t, db, initialPolicy())
		db.setGet(func(context.Context, string) error { return errors.New("private database connection details") })
		admin := policyActor().Identity
		response := managementRequest(t, managementHandler(runtime, ""), &admin, http.MethodPut,
			"/providers/Radius.Core/roleAssignments/reader"+managementVersion, assignmentBody(builtInID("reader"), "bob"))
		require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), "private")
	})
}

func TestManagementReadRechecksFreshness(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancelled", true: "expired"}[expire], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			cache, err := NewPolicyCache(policySourceFunc(func(context.Context, int64) (*PolicySnapshot, error) {
				return nil, errors.New("unexpected read")
			}), evaluationClock{WithTicker: clock.RealClock{}, since: func(time.Time) time.Duration {
				calls++
				if calls == 2 {
					if expire {
						return PolicyMaxAge
					}
					cancel()
				}
				return 0
			}})
			require.NoError(t, err)
			catalog, err := NewCatalog(nil)
			require.NoError(t, err)
			cache.value, err = NewPolicySnapshot(catalog, initialPolicy())
			require.NoError(t, err)
			runtime := &PolicyRuntime{ctx: ctx, cache: cache}
			target, matched, err := managementRoute("/providers/Radius.Core/roleAssignments")
			require.NoError(t, err)
			require.True(t, matched)
			result, err := runtime.readManagement(policyActor().Identity, target, true)
			require.ErrorIs(t, err, ErrAuthorizationUnavailable)
			require.Nil(t, result)
		})
	}
}
