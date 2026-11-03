// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

func TestInspectionEveryMode(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store := newTestPolicyStore(t, inmemory.NewClient(), false)
				policy := initialPolicy()
				policy.Settings.Properties.Mode = &mode
				init, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
				require.NoError(t, err)
				require.NoError(t, init.Initialize(t.Context(), policy))
				runtime, _ := startPolicyRuntime(t, store)
				admin := policyActor().Identity
				reader := Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob")}
				groupMember := Identity{Principal: reader.Principal, Groups: admin.Groups}
				for _, identity := range []Identity{admin, reader, groupMember} {
					current, err := runtime.WhoAmI(identity)
					require.NoError(t, err)
					require.Equal(t, mode, *current.Mode)
					require.Equal(t, *identity.Principal.Subject != "bob" || len(identity.Groups) != 0, *current.IsRadiusAdministrator)
					require.Len(t, current.Warnings, 2)
					// Responses must not expose mutable policy pointers.
					*current.Mode = api.UserAuthorizationMode("tampered")
					require.Equal(t, mode, runtime.State().LastKnownMode)
				}
				checks := []*api.AccessCheck{
					{Permission: to.Ptr("RADIUS.CORE/APPLICATIONS/READ"), Target: to.Ptr(strings.ToUpper(appA))},
					{Permission: to.Ptr("Radius.Core/applications/write"), Target: to.Ptr(appA)},
				}
				for _, identity := range []Identity{reader, groupMember} {
					result, err := runtime.CheckAccess(identity, "/", api.AccessCheckRequest{Checks: checks})
					require.NoError(t, err)
					require.Equal(t, mode, *result.Mode)
					require.Equal(t, int64(0), *result.Generation)
					require.Len(t, result.Checks, 2)
					for i, check := range result.Checks {
						require.Equal(t, len(identity.Groups) != 0, *check.Allowed)
						require.Equal(t, checks[i].Permission, check.Permission)
						require.Equal(t, checks[i].Target, check.Target)
					}
					self, err := runtime.CheckAccess(identity, "/", api.AccessCheckRequest{Checks: checks, Principal: &identity.Principal})
					require.NoError(t, err)
					require.Equal(t, result, self)
				}
				for _, principal := range []api.Principal{admin.Principal, admin.Groups[0]} {
					_, err := runtime.CheckAccess(reader, "/", api.AccessCheckRequest{Checks: checks, Principal: &principal})
					require.ErrorIs(t, err, ErrCheckAccessDenied)
				}
				_, err = runtime.CheckAccess(admin, "/", api.AccessCheckRequest{Checks: checks, Principal: &reader.Principal})
				require.ErrorIs(t, err, ErrMembershipUnavailable)
				otherGroup, err := runtime.CheckAccess(admin, "/", api.AccessCheckRequest{Checks: checks, Principal: &admin.Groups[0]})
				require.NoError(t, err)
				require.True(t, *otherGroup.Checks[0].Allowed)
				for _, request := range []api.AccessCheckRequest{
					{},
					{Checks: []*api.AccessCheck{nil}},
					{Checks: []*api.AccessCheck{{}}},
					{Checks: []*api.AccessCheck{{Permission: to.Ptr("not/a/permission"), Target: to.Ptr(appA)}}},
					{Checks: []*api.AccessCheck{{Permission: checks[0].Permission, Target: to.Ptr(appA + "/../escape")}}},
					{Checks: checks, Principal: &api.Principal{}},
				} {
					_, err := runtime.CheckAccess(admin, "/", request)
					require.Error(t, err)
				}
				_, err = runtime.CheckAccess(admin, appA+"/child", api.AccessCheckRequest{Checks: checks})
				require.Error(t, err)
				_, err = runtime.CheckAccess(admin, teamB, api.AccessCheckRequest{Checks: checks})
				require.ErrorContains(t, err, "within the inspection scope")

				for _, identified := range []bool{false, true} {
					for _, path := range []string{"/providers/Radius.Core/whoAmI", "/providers/Radius.Core/checkAccess"} {
						method, body := http.MethodGet, ""
						if strings.HasSuffix(path, "checkAccess") {
							method, body = http.MethodPost, `{"checks":[{"permission":"Radius.Core/applications/read","target":"/"}]}`
						}
						req := httptest.NewRequest(method, path+"?api-version=2025-08-01-preview", strings.NewReader(body))
						if identified {
							req = req.WithContext(context.WithValue(req.Context(), identityContextKey{}, admin))
						}
						response := httptest.NewRecorder()
						ObserveRequests(runtime, "")(Inspection(runtime, "")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
							t.Fatal("inspection fell through to generic routing")
						}))).ServeHTTP(response, req)
						expected := http.StatusUnauthorized
						if identified {
							expected = http.StatusOK
						}
						require.Equal(t, expected, response.Code, response.Body.String())
					}
				}
			})
		})
	}
}

func TestInspectionUnavailable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newTestPolicyStore(t, inmemory.NewClient(), true)
		active, stop := startPolicyRuntime(t, store)
		stopped, stopOther := startPolicyRuntime(t, store)
		stopOther()
		missing, _ := startPolicyRuntime(t, newTestPolicyStore(t, inmemory.NewClient(), false))
		for _, runtime := range []*PolicyRuntime{nil, missing, stopped} {
			result, err := runtime.WhoAmI(policyActor().Identity)
			require.ErrorIs(t, err, ErrAuthorizationUnavailable)
			require.Nil(t, result.Mode)
			batch, err := runtime.CheckAccess(policyActor().Identity, "/", api.AccessCheckRequest{})
			require.ErrorIs(t, err, ErrAuthorizationUnavailable)
			require.Nil(t, batch.Mode)
		}
		// Exact expiry without allowing a background refresh to restore freshness.
		active.cache.mu.Lock()
		active.cache.asOf = active.cache.asOf.Add(-PolicyMaxAge)
		active.cache.mu.Unlock()
		_, err := active.WhoAmI(policyActor().Identity)
		require.ErrorIs(t, err, ErrAuthorizationUnavailable)
		stop()
	})
}

func TestInspectionBatchGeneration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime, _ := startPolicyRuntime(t, newTestPolicyStore(t, inmemory.NewClient(), true))
		bob := Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob")}
		request := api.AccessCheckRequest{Checks: []*api.AccessCheck{
			{Permission: to.Ptr("Radius.Core/applications/read"), Target: to.Ptr(appA)},
			{Permission: to.Ptr("Radius.Core/applications/read"), Target: to.Ptr(appA)},
		}}
		var writer sync.WaitGroup
		writer.Go(func() {
			for range 10 {
				mutation := readerMutation()
				if err := runtime.Mutate(t.Context(), policyActor(), mutation); err != nil {
					t.Error(err)
					return
				}
				mutation.Delete, mutation.Assignment = true, nil
				if err := runtime.Mutate(t.Context(), policyActor(), mutation); err != nil {
					t.Error(err)
					return
				}
			}
		})
		for range 200 {
			result, err := runtime.CheckAccess(bob, "/", request)
			require.NoError(t, err)
			for _, check := range result.Checks {
				require.Equal(t, *result.Generation%2 == 1, *check.Allowed, "generation %d", *result.Generation)
			}
		}
		writer.Wait()
		require.Equal(t, int64(20), runtime.State().Generation)
	})
}

func TestInspectionBootstrapWarning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newTestPolicyStore(t, inmemory.NewClient(), false)
		policy := initialPolicy()
		for _, assignment := range policy.Assignments {
			assignment.Properties.ManagedBy.Kind = to.Ptr(api.AssignmentManagementKindBootstrap)
		}
		initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
		require.NoError(t, err)
		require.NoError(t, initializer.Initialize(t.Context(), policy))
		runtime, _ := startPolicyRuntime(t, store)
		result, err := runtime.WhoAmI(policyActor().Identity)
		require.NoError(t, err)
		require.True(t, *result.IsRadiusAdministrator)
		require.Len(t, result.Warnings, 3)
		require.Contains(t, *result.Warnings[1], "No permanent user or group installation administrator")
		for _, warning := range result.Warnings {
			require.NotContains(t, *warning, "alpha")
		}
	})
}

func TestInspectionHTTPValidation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime, _ := startPolicyRuntime(t, newTestPolicyStore(t, inmemory.NewClient(), true))
		valid := `{"checks":[{"permission":"Radius.Core/applications/read","target":"/"}]}`
		for _, tt := range []struct {
			name, method, path, body string
			status                   int
		}{
			{"valid", "POST", "/providers/RADIUS.CORE/CHECKACCESS?api-version=2025-08-01-preview", valid, 200},
			{"scope invalid", "POST", "/bad/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", valid, 400},
			{"spoof groups", "POST", "/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", `{"groups":["admins"],"checks":[]}`, 400},
			{"nested groups", "POST", "/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", `{"principal":{"groups":["admins"]},"checks":[]}`, 400},
			{"unknown check field", "POST", "/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", `{"checks":[{"groups":["admins"]}]}`, 400},
			{"empty", "POST", "/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", `{"checks":[]}`, 400},
			{"null", "POST", "/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", `null`, 400},
			{"multiple", "POST", "/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", valid + valid, 400},
			{"oversize", "POST", "/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", `{"checks":[], "extra":"` + strings.Repeat("x", 1<<20) + `"}`, 400},
			{"invalid JSON", "POST", "/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", "{", 400},
			{"missing version", "GET", "/providers/Radius.Core/whoAmI", "", 400},
			{"wrong version", "GET", "/providers/Radius.Core/whoAmI?api-version=unknown", "", 400},
			{"method", "POST", "/providers/Radius.Core/whoAmI?api-version=2025-08-01-preview", "", 405},
		} {
			{
				request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				request = request.WithContext(context.WithValue(request.Context(), identityContextKey{}, policyActor().Identity))
				response := httptest.NewRecorder()
				Inspection(runtime, "")(http.NotFoundHandler()).ServeHTTP(response, request)
				require.Equal(t, tt.status, response.Code, "%s: %s", tt.name, response.Body.String())
				var wire map[string]any
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &wire))
				if tt.status != 200 {
					require.Contains(t, wire, "error")
				} else {
					require.NotContains(t, response.Body.String(), "assignment")
					require.Contains(t, wire, "generation")
				}
			}
		}
	})
}
