// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"k8s.io/utils/clock"
	clocktesting "k8s.io/utils/clock/testing"
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
			batch, err := runtime.CheckAccess(policyActor().Identity, "/", api.AccessCheckRequest{
				Checks: []*api.AccessCheck{{Permission: to.Ptr("Radius.Core/applications/read"), Target: to.Ptr("/")}},
			})
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

func TestInspectionSnapshotAvailabilityChanges(t *testing.T) {
	for _, scenario := range []string{"expired", "refreshed after expiry", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			clk := clocktesting.NewFakeClock(time.Now())
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			snapshot := testPolicySnapshot(t, 1)
			runtime := &PolicyRuntime{ctx: ctx, cache: &PolicyCache{value: snapshot, asOf: clk.Now(), clock: clk}}
			err := runtime.inspectPolicy(func(captured *PolicySnapshot) error {
				require.Same(t, snapshot, captured)
				require.True(t, runtime.mu.TryLock(), "inspection must not hold the runtime lock")
				runtime.mu.Unlock()
				require.True(t, runtime.cache.mu.TryLock(), "inspection must not block refresh")
				defer runtime.cache.mu.Unlock()
				if scenario == "cancelled" {
					cancel()
				} else {
					clk.Step(PolicyMaxAge)
					if scenario == "refreshed after expiry" {
						runtime.cache.value = testPolicySnapshot(t, 2)
						runtime.cache.asOf = clk.Now()
					}
				}
				return nil
			})
			require.ErrorIs(t, err, ErrAuthorizationUnavailable)
		})
	}
	for _, operation := range []string{"whoami", "checkAccess"} {
		for _, scenario := range []string{"expired", "cancelled"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				calls := 0
				clk := evaluationClock{WithTicker: clock.RealClock{}, since: func(time.Time) time.Duration {
					calls++
					if calls == 2 {
						if scenario == "cancelled" {
							cancel()
						} else {
							return PolicyMaxAge
						}
					}
					return 0
				}}
				runtime := &PolicyRuntime{ctx: ctx, cache: &PolicyCache{value: testPolicySnapshot(t, 1), asOf: time.Now(), clock: clk}}
				method, body, route := http.MethodGet, "", "whoAmI"
				if operation == "checkAccess" {
					method, body, route = http.MethodPost, `{"checks":[{"permission":"Radius.Core/applications/read","target":"/"}]}`, "checkAccess"
				}
				request := httptest.NewRequest(method, "/providers/Radius.Core/"+route+"?api-version=2025-08-01-preview", strings.NewReader(body))
				request = request.WithContext(context.WithValue(request.Context(), identityContextKey{}, policyActor().Identity))
				response := httptest.NewRecorder()
				Inspection(runtime, "")(http.NotFoundHandler()).ServeHTTP(response, request)
				require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
				require.Contains(t, response.Body.String(), "AuthorizationUnavailable")
				require.NotContains(t, response.Body.String(), `"checks"`)
				require.NotContains(t, response.Body.String(), `"principal"`)
			})
		}
	}
}

func TestInspectionInputLimits(t *testing.T) {
	snapshot := testPolicySnapshot(t, 1)
	longPermission := "Test.Long/" + strings.Repeat("x", maxPermissionLength-len("Test.Long//read")) + "/read"
	snapshot.evaluator.catalog.permissions[strings.ToLower(longPermission)] = longPermission
	runtime := &PolicyRuntime{ctx: t.Context(), cache: &PolicyCache{value: snapshot, asOf: time.Now(), clock: clock.RealClock{}}}
	for _, field := range []struct {
		name  string
		limit int
		set   func(*api.AccessCheckRequest, *string, int)
	}{
		{"checks", maxAccessChecks, func(r *api.AccessCheckRequest, _ *string, n int) {
			check := r.Checks[0]
			r.Checks = make([]*api.AccessCheck, n)
			for i := range r.Checks {
				r.Checks[i] = check
			}
		}},
		{"principal.issuer", maxPrincipalIssuerLength, func(r *api.AccessCheckRequest, _ *string, n int) { r.Principal.Issuer = to.Ptr(strings.Repeat("i", n)) }},
		{"principal.subject", maxPrincipalSubjectLength, func(r *api.AccessCheckRequest, _ *string, n int) {
			r.Principal.Subject = to.Ptr(strings.Repeat("s", n))
		}},
		{"principal.displayName", maxPrincipalDisplayNameLength, func(r *api.AccessCheckRequest, _ *string, n int) {
			r.Principal.DisplayName = to.Ptr(strings.Repeat("\u754c", n))
		}},
		{"permission", maxPermissionLength, func(r *api.AccessCheckRequest, _ *string, n int) {
			r.Checks[0].Permission = to.Ptr(longPermission + strings.Repeat("x", n-maxPermissionLength))
		}},
		{"target", maxAccessScopeLength, func(r *api.AccessCheckRequest, _ *string, n int) {
			r.Checks[0].Target = to.Ptr("/planes/radius/" + strings.Repeat("t", n-len("/planes/radius/")))
		}},
		{"scope", maxAccessScopeLength, func(r *api.AccessCheckRequest, scope *string, n int) {
			*scope = "/planes/radius/" + strings.Repeat("s", n-len("/planes/radius/"))
			r.Checks[0].Target = scope
		}},
	} {
		for _, above := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/above=%t", field.name, above), func(t *testing.T) {
				request := api.AccessCheckRequest{
					Principal: to.Ptr(testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-a", "other-group")),
					Checks:    []*api.AccessCheck{{Permission: to.Ptr("Radius.Core/applications/read"), Target: to.Ptr("/")}},
				}
				scope, n := "/", field.limit
				if above {
					n++
				}
				field.set(&request, &scope, n)
				result, err := runtime.CheckAccess(policyActor().Identity, scope, request)
				if above {
					require.ErrorContains(t, err, field.name)
					require.Nil(t, result.Checks)
					// Even an unconfigured runtime must reject oversized input
					// before inspecting policy or trying to acquire its locks.
					_, err = (*PolicyRuntime)(nil).CheckAccess(policyActor().Identity, scope, request)
					require.ErrorContains(t, err, field.name)
				} else {
					require.NoError(t, err)
					require.Len(t, result.Checks, len(request.Checks))
				}
				body, err := json.Marshal(request)
				require.NoError(t, err)
				pathScope := strings.TrimSuffix(scope, "/")
				req := httptest.NewRequest(http.MethodPost, pathScope+"/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", strings.NewReader(string(body)))
				req = req.WithContext(context.WithValue(req.Context(), identityContextKey{}, policyActor().Identity))
				response := httptest.NewRecorder()
				handlerRuntime := runtime
				if above {
					handlerRuntime = nil
				}
				Inspection(handlerRuntime, "")(http.NotFoundHandler()).ServeHTTP(response, req)
				if above {
					require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), "InvalidRequestContent")
					require.Contains(t, response.Body.String(), field.name)
				} else {
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				}
			})
		}
	}
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

func TestInspectionContractLimits(t *testing.T) {
	data, err := os.ReadFile("../../../swagger/specification/authorization/resource-manager/Radius.Core/preview/2025-08-01-preview/openapi.json")
	require.NoError(t, err)
	type schema struct {
		MaxLength  int `json:"maxLength"`
		Properties map[string]struct {
			MaxItems  int `json:"maxItems"`
			MaxLength int `json:"maxLength"`
		} `json:"properties"`
	}
	var document struct {
		Definitions map[string]schema `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(data, &document))
	for _, field := range []struct {
		model, property string
		limit           int
	}{
		{"Principal", "issuer", maxPrincipalIssuerLength},
		{"Principal", "subject", maxPrincipalSubjectLength},
		{"Principal", "displayName", maxPrincipalDisplayNameLength},
		{"AccessCheck", "target", maxAccessScopeLength},
	} {
		require.Equal(t, field.limit, document.Definitions[field.model].Properties[field.property].MaxLength, "%s.%s", field.model, field.property)
	}
	require.Equal(t, maxAccessChecks, document.Definitions["AccessCheckRequest"].Properties["checks"].MaxItems)
	require.Equal(t, maxPermissionLength, document.Definitions["PermissionName"].MaxLength)
	require.Equal(t, maxAccessScopeLength, document.Definitions["AuthorizationScope"].MaxLength)
	require.Equal(t, maxAccessScopeLength-1, document.Definitions["AuthorizationScopePath"].MaxLength)
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
			{"oversize principal type", "POST", "/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview", `{"principal":{"type":"` + strings.Repeat("x", 1024) + `","issuer":"cluster","subject":"other"},"checks":[{"permission":"Radius.Core/applications/read","target":"/"}]}`, 400},
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

func TestInspectionUnicodeScope(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Kteam", "\u212ateam", "\u0130team", "\u1e9eteam"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scope := "/planes/radius/local/resourceGroups/" + name
			path := scope + "/providers/Radius.Core/checkAccess"
			operation, actualScope := inspectionRoute(path)
			require.Equal(t, "checkaccess", operation)
			require.Equal(t, strings.ToLower(scope), actualScope)
			runtime := &PolicyRuntime{ctx: t.Context(), cache: &PolicyCache{
				value: testPolicySnapshot(t, 1), asOf: time.Now(), clock: clock.RealClock{},
			}}
			for _, target := range []string{scope, scope + "/providers/Radius.Core/applications/app", scope + "-neighbor"} {
				body, err := json.Marshal(api.AccessCheckRequest{Checks: []*api.AccessCheck{
					{Permission: to.Ptr("Radius.Core/applications/read"), Target: to.Ptr(target)},
				}})
				require.NoError(t, err)
				request := httptest.NewRequest(http.MethodPost, path+"?api-version=2025-08-01-preview", strings.NewReader(string(body)))
				request = request.WithContext(context.WithValue(request.Context(), identityContextKey{}, policyActor().Identity))
				response := httptest.NewRecorder()
				Inspection(runtime, "")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Fatal("inspection fell through to generic routing")
				})).ServeHTTP(response, request)
				if strings.HasSuffix(target, "-neighbor") {
					require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), "within the inspection scope")
					continue
				}
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var result api.CheckAccessResponse
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
				require.Len(t, result.Checks, 1)
				require.True(t, *result.Checks[0].Allowed)
				require.Equal(t, target, *result.Checks[0].Target)
			}
		})
	}
}

func TestInspectionAPIVersionDoesNotBypassAuthorization(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{
		api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce,
	} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store := newTestPolicyStore(t, inmemory.NewClient(), false)
				policy := initialPolicy()
				policy.Settings.Properties.Mode = &mode
				initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
				require.NoError(t, err)
				require.NoError(t, initializer.Initialize(t.Context(), policy))
				runtime, _ := startPolicyRuntime(t, store)
				for _, version := range []string{"", "?api-version=unknown", "?api-version=2025-08-01-preview"} {
					for _, verified := range []bool{false, true} {
						body := `{"principal":{"type":"group","issuer":"kubernetes:cluster-a","subject":"admins"},"checks":[{"permission":"Radius.Core/applications/read","target":"/"}]}`
						request := httptest.NewRequest(http.MethodPost, "/providers/Radius.Core/checkAccess"+version, strings.NewReader(body))
						if verified {
							caller := Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob")}
							request = request.WithContext(context.WithValue(request.Context(), identityContextKey{}, caller))
						}
						response := httptest.NewRecorder()
						Inspection(runtime, "")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
							t.Fatal("inspection fell through to generic routing")
						})).ServeHTTP(response, request)
						status, code := http.StatusUnauthorized, "InvalidAuthenticationInfo"
						if verified {
							status, code = http.StatusBadRequest, "InvalidApiVersionParameter"
							if version == "?api-version=2025-08-01-preview" {
								status, code = http.StatusForbidden, "AuthorizationFailed"
							}
						}
						require.Equal(t, status, response.Code, response.Body.String())
						require.Contains(t, response.Body.String(), code)
						require.NotContains(t, response.Body.String(), `"checks"`)
					}
				}
			})
		})
	}
}

func TestInspectionInvalidCaller(t *testing.T) {
	t.Parallel()
	runtime := &PolicyRuntime{ctx: t.Context(), cache: &PolicyCache{
		value: testPolicySnapshot(t, 1), asOf: time.Now(), clock: clock.RealClock{},
	}}
	checks := []*api.AccessCheck{{Permission: to.Ptr("Radius.Core/applications/read"), Target: to.Ptr("/")}}
	for _, tt := range []struct {
		name     string
		identity Identity
	}{
		{"missing principal", Identity{}},
		{"malformed group", Identity{Principal: testIdentity().Principal, Groups: []api.Principal{{}}}},
		{"wrong group issuer", Identity{Principal: testIdentity().Principal, Groups: []api.Principal{
			testPrincipal(api.PrincipalTypeGroup, "kubernetes:another", "admins"),
		}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			current, err := runtime.WhoAmI(tt.identity)
			require.Error(t, err)
			require.Nil(t, current.Principal)
			require.Nil(t, current.Mode)
			result, err := runtime.CheckAccess(tt.identity, "/", api.AccessCheckRequest{Checks: checks})
			require.Error(t, err)
			require.Nil(t, result.Checks)
			result, err = runtime.CheckAccess(tt.identity, "/", api.AccessCheckRequest{
				Checks: checks, Principal: to.Ptr(policyActor().Identity.Groups[0]),
			})
			require.Error(t, err)
			require.Nil(t, result.Checks)
		})
	}
}

func TestInspectionRoutingAndPermissionErrors(t *testing.T) {
	t.Parallel()
	runtime := &PolicyRuntime{ctx: t.Context(), cache: &PolicyCache{
		value: testPolicySnapshot(t, 1), asOf: time.Now(), clock: clock.RealClock{},
	}}
	const prefix = "/apis/api.ucp.dev/v1alpha3"
	for _, tt := range []struct {
		name, path, body string
		caller           Identity
		status           int
		code             string
	}{
		{"outside path base", "/providers/Radius.Core/whoAmI", "", Identity{}, 204, ""},
		{"unknown operation", prefix + "/providers/Radius.Core/unknown", "", Identity{}, 204, ""},
		{"group inspection denied", prefix + "/providers/Radius.Core/checkAccess",
			`{"principal":{"type":"group","issuer":"kubernetes:cluster-a","subject":"admins"},"checks":[{"permission":"Radius.Core/applications/read","target":"/"}]}`,
			Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob")}, 403, "AuthorizationFailed"},
		{"other membership unavailable", prefix + "/providers/Radius.Core/checkAccess",
			`{"principal":{"type":"user","issuer":"kubernetes:cluster-a","subject":"bob"},"checks":[{"permission":"Radius.Core/applications/read","target":"/"}]}`,
			policyActor().Identity, 503, "AuthorizationIndeterminate"},
		{"null check", prefix + "/providers/Radius.Core/checkAccess", `{"checks":[null]}`, policyActor().Identity, 400, "InvalidRequestContent"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path+"?api-version=2025-08-01-preview", strings.NewReader(tt.body))
			req = req.WithContext(context.WithValue(req.Context(), identityContextKey{}, tt.caller))
			response := httptest.NewRecorder()
			fellThrough := false
			Inspection(runtime, prefix)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fellThrough = true
				require.Equal(t, req.URL.Path, r.URL.Path, "routing must preserve the incoming path")
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(response, req)
			require.Equal(t, tt.status, response.Code, response.Body.String())
			require.Equal(t, tt.status == 204, fellThrough)
			if tt.code != "" {
				require.Contains(t, response.Body.String(), tt.code)
				require.NotContains(t, response.Body.String(), `"checks"`)
			}
		})
	}
}

type inspectionFailingWriter struct {
	header http.Header
	status int
}

func (w *inspectionFailingWriter) Header() http.Header    { return w.header }
func (w *inspectionFailingWriter) WriteHeader(status int) { w.status = status }
func (w *inspectionFailingWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("response disconnected")
}

func TestInspectionResponseWriteErrors(t *testing.T) {
	t.Parallel()
	runtime := &PolicyRuntime{ctx: t.Context(), cache: &PolicyCache{
		value: testPolicySnapshot(t, 1), asOf: time.Now(), clock: clock.RealClock{},
	}}
	for _, verified := range []bool{false, true} {
		t.Run(fmt.Sprintf("verified=%t", verified), func(t *testing.T) {
			core, entries := observer.New(zap.ErrorLevel)
			ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))
			if verified {
				ctx = context.WithValue(ctx, identityContextKey{}, policyActor().Identity)
			}
			req := httptest.NewRequest(http.MethodGet, "/providers/Radius.Core/whoAmI?api-version=2025-08-01-preview", nil).WithContext(ctx)
			writer := &inspectionFailingWriter{header: http.Header{}}
			Inspection(runtime, "")(http.NotFoundHandler()).ServeHTTP(writer, req)
			message := "Failed to write inspection error"
			if verified {
				message = "Failed to write inspection response"
			}
			require.Len(t, entries.FilterMessage(message).All(), 1)
			require.Contains(t, entries.All()[0].ContextMap()["error"], "response disconnected")
			require.Equal(t, "application/json", writer.Header().Get("Content-Type"))
			require.Equal(t, 0, entries.FilterMessage("request").Len())
			require.NotContains(t, entries.All()[0].ContextMap(), "resourceId")
		})
	}
}
