// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"k8s.io/utils/clock"
)

func requestRuntime(t *testing.T, mode api.UserAuthorizationMode) (*PolicyRuntime, *runtimeDatabase, func()) {
	t.Helper()
	db := &runtimeDatabase{Client: inmemory.NewClient()}
	store := newTestPolicyStore(t, db, false)
	policy := initialPolicy()
	policy.Settings.Properties.Mode = &mode
	initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
	require.NoError(t, err)
	require.NoError(t, initializer.Initialize(t.Context(), policy))
	runtime, stop := startPolicyRuntime(t, store)
	return runtime, db, stop
}

func TestObserveRequests(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, subject, result string
	}{
		{"allowed target", http.MethodPut, appA, "alice", "allow"},
		{"denied target", http.MethodPut, appA, "nobody", "wouldDeny"},
		{"unknown target", http.MethodPut, teamA + "/providers/Example.Test/widgets/a", "alice", "indeterminate"},
		{"missing identity", http.MethodGet, appA, "", "indeterminate"},
		{"malformed target", http.MethodGet, appA + "/../b", "alice", "indeterminate"},
		{"denied list needs filtering", http.MethodGet, teamA + "/providers/Radius.Core/applications", "nobody", "indeterminate"},
		{"mixed list needs filtering", http.MethodGet, teamA + "/resources", "alice", "indeterminate"},
		{"operation needs original context", http.MethodGet, "/planes/radius/local/providers/System.Resources/locations/local/operationStatuses/abc", "alice", "indeterminate"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runtime, _, stop := requestRuntime(t, api.UserAuthorizationModeDryRun)
				defer stop()
				core, logs := observer.New(zap.InfoLevel)
				logs.TakeAll()
				body := `{"properties":{"secret":"must-not-be-logged"}}`
				request := httptest.NewRequest(tt.method, "/apis/api.ucp.dev/v1alpha3"+tt.path+"?api-version=secret-query", strings.NewReader(body))
				request.Header.Set("X-Test", "unchanged")
				ctx := logr.NewContext(request.Context(), zapr.NewLogger(zap.New(core)))
				if tt.subject != "" {
					identity := testIdentity()
					identity.Principal.Subject = &tt.subject
					ctx = context.WithValue(ctx, identityContextKey{}, identity)
				}
				request = request.WithContext(ctx)
				response := httptest.NewRecorder()
				called := false
				handler := ObserveRequests(runtime, "/apis/api.ucp.dev/v1alpha3")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					actual, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.Equal(t, body, string(actual))
					require.Equal(t, "api-version=secret-query", r.URL.RawQuery)
					require.Equal(t, "unchanged", r.Header.Get("X-Test"))
					w.Header().Set("X-Downstream", "preserved")
					w.WriteHeader(http.StatusAccepted)
					_, err = w.Write([]byte("original response"))
					require.NoError(t, err)
				}))
				handler.ServeHTTP(response, request)
				require.True(t, called)
				require.Equal(t, http.StatusAccepted, response.Code)
				require.Equal(t, "original response", response.Body.String())
				require.Equal(t, "preserved", response.Header().Get("X-Downstream"))
				entries := logs.FilterMessage("User authorization request").All()
				require.Len(t, entries, 1)
				fields := entries[0].ContextMap()
				require.Equal(t, tt.result, fields["result"])
				require.Equal(t, "DryRun", fields["mode"])
				require.Equal(t, int64(0), fields["generation"])
				require.Equal(t, "targetOnly", fields["coverage"])
				encoded, err := json.Marshal(fields)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "must-not-be-logged")
				require.NotContains(t, string(encoded), "secret-query")
				require.NotContains(t, fields, "grantingAssignment")
				require.NotContains(t, fields, "roleDefinitionId")
			})
		})
	}
}

func TestObserveRequestsAvailability(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{
		api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce,
	} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runtime, db, stop := requestRuntime(t, mode)
				core, logs := observer.New(zap.InfoLevel)
				calls := 0
				handler := ObserveRequests(runtime, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.WriteHeader(http.StatusNoContent)
				}))
				run := func(method, path string) *httptest.ResponseRecorder {
					request := httptest.NewRequest(method, path, nil)
					request = request.WithContext(logr.NewContext(request.Context(), zapr.NewLogger(zap.New(core))))
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)
					return response
				}
				fresh := run(http.MethodPut, appA)
				if mode == api.UserAuthorizationModeEnforce {
					require.Equal(t, http.StatusServiceUnavailable, fresh.Code)
					require.Zero(t, calls)
				} else {
					require.Equal(t, http.StatusNoContent, fresh.Code)
				}
				if mode == api.UserAuthorizationModeDisabled {
					require.Zero(t, logs.Len())
				}
				require.Equal(t, http.StatusNoContent, run(http.MethodGet, "/version").Code)
				db.setGet(func(context.Context, string) error { return errors.New("offline") })
				time.Sleep(PolicyMaxAge - time.Nanosecond)
				synctest.Wait()
				require.Equal(t, RuntimeFresh, runtime.State().Status)
				time.Sleep(time.Nanosecond)
				synctest.Wait()
				for _, state := range []string{"stale", "cancelled"} {
					if state == "cancelled" {
						stop()
					}
					logs.TakeAll()
					require.Equal(t, http.StatusNoContent, run(http.MethodPut, appA).Code)
					entries := logs.FilterMessage("User authorization request").All()
					require.Len(t, entries, 1)
					fields := entries[0].ContextMap()
					require.Equal(t, "indeterminate", fields["result"])
					require.Equal(t, "", fields["mode"])
					require.Equal(t, int64(-1), fields["generation"])
					require.Equal(t, "AuthorizationUnavailable", fields["error"])
					result, err := runtime.EvaluateRequest(testIdentity(), http.MethodGet, appA)
					require.ErrorIs(t, err, ErrAuthorizationUnavailable)
					require.Empty(t, result.Mode)
					require.Nil(t, result.Decisions)
					for _, verified := range []bool{false, true} {
						request := httptest.NewRequest(http.MethodPut, "/providers/Radius.Core/authorizationSettings/default", nil)
						if verified {
							request = request.WithContext(context.WithValue(request.Context(), identityContextKey{}, testIdentity()))
						}
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, request)
						expected := http.StatusUnauthorized
						if verified {
							expected = http.StatusServiceUnavailable
						}
						require.Equal(t, expected, response.Code)
					}
				}
				db.setGet(nil)
				policy, err := runtime.store.LoadPolicy(t.Context(), -1)
				require.NoError(t, err)
				require.Equal(t, int64(0), policy.Generation())
			})
		})
	}
}

func TestObserveRequestsProtectedMutations(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{
		api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce,
	} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runtime, _, stop := requestRuntime(t, mode)
				defer stop()
				for _, active := range []*PolicyRuntime{nil, runtime} {
					for _, verified := range []bool{false, true} {
						for _, target := range []string{
							"/providers/Radius.Core/roleDefinitions/a",
							teamA + "/providers/Radius.Core/roleAssignments/a",
							"/providers/Radius.Core/authorizationSettings/default",
							"/providers/Radius.Core/accessChanges/1",
							"/providers/System.Authorization/accessVersions/default",
							teamA + "/providers/System.Authorization/policyObjects/a",
							teamA + "/providers/RADIUS.CORE/roleAssignments/a/unknownAction",
							teamA + "/providers/Radius.Core/roleDefinitions",
							teamA + "/providers/Radius.Core/roleAssignments/a/../b",
						} {
							for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodPost} {
								request := httptest.NewRequest(method, target, strings.NewReader(`{"properties":{}}`))
								if verified {
									request = request.WithContext(context.WithValue(request.Context(), identityContextKey{}, testIdentity()))
								}
								response := httptest.NewRecorder()
								ObserveRequests(active, "")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
									t.Fatal("protected mutation reached generic/proxy handler")
								})).ServeHTTP(response, request)
								if verified {
									require.Equal(t, http.StatusServiceUnavailable, response.Code, target)
									require.Contains(t, response.Body.String(), "AuthorizationUnavailable")
								} else {
									require.Equal(t, http.StatusUnauthorized, response.Code, target)
								}
							}
						}
					}
				}
				snapshot, err := runtime.store.LoadPolicy(t.Context(), -1)
				require.NoError(t, err)
				require.Equal(t, int64(0), snapshot.Generation())
				require.Equal(t, mode, *snapshot.policy.Settings.Properties.Mode)
				require.Equal(t, initialPolicy().Assignments, snapshot.policy.Assignments)
			})
		})
	}
}

func TestObserveRequestsNotConfiguredAndNeverLoaded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newTestPolicyStore(t, inmemory.NewClient(), false)
		runtime, stop := startPolicyRuntime(t, store)
		defer stop()
		for _, active := range []*PolicyRuntime{nil, runtime} {
			core, logs := observer.New(zap.InfoLevel)
			request := httptest.NewRequest(http.MethodGet, appA, nil)
			request = request.WithContext(logr.NewContext(request.Context(), zapr.NewLogger(zap.New(core))))
			response := httptest.NewRecorder()
			ObserveRequests(active, "")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(response, request)
			require.Equal(t, http.StatusNoContent, response.Code)
			if active == nil {
				require.Zero(t, logs.Len())
			} else {
				require.Equal(t, "indeterminate", logs.All()[0].ContextMap()["result"])
			}
		}
		_, err := store.LoadPolicy(t.Context(), -1)
		require.ErrorIs(t, err, ErrAuthorizationUnavailable)
	})
}

func TestPolicyRuntimeRequestGeneration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime, _, stop := requestRuntime(t, api.UserAuthorizationModeDryRun)
		defer stop()
		identity := testIdentity()
		identity.Principal.Subject = new("bob")
		// Generation zero denies Bob; generation one grants Reader. A mode,
		// generation, and decision must never be assembled from separate reads.
		before, err := runtime.EvaluateRequest(identity, http.MethodGet, appA)
		require.NoError(t, err)
		require.False(t, before.Decisions[0].Allowed)
		results := make(chan RequestEvaluation, 400)
		failures := make(chan error, 400)
		var readers sync.WaitGroup
		for range 8 {
			readers.Go(func() {
				for range 50 {
					result, err := runtime.EvaluateRequest(identity, http.MethodGet, appA)
					if err != nil {
						failures <- err
					}

					results <- result
				}
			})
		}
		require.NoError(t, runtime.Mutate(t.Context(), policyActor(), readerMutation()))
		readers.Wait()
		close(results)
		close(failures)
		for err := range failures {
			require.NoError(t, err)
		}
		for result := range results {
			require.Equal(t, api.UserAuthorizationModeDryRun, result.Mode)
			require.Len(t, result.Decisions, 1)
			require.Contains(t, []int64{0, 1}, result.Generation)
			require.Equal(t, result.Generation == 1, result.Decisions[0].Allowed)
		}
		after, err := runtime.EvaluateRequest(identity, http.MethodGet, appA)
		require.NoError(t, err)
		require.Equal(t, int64(1), after.Generation)
		require.True(t, after.Decisions[0].Allowed)
	})
}

func TestPolicyRuntimeEvaluateRequestAvailabilityChanges(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{
		api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce,
	} {
		for _, path := range []string{appA, "/version", "/unknown"} {
			for _, scenario := range []string{"cancelled", "expired"} {
				t.Run(string(mode)+path+"/"+scenario, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					snapshot := testPolicySnapshot(t, 1)
					snapshot.policy.Settings.Properties.Mode = &mode
					runtime := &PolicyRuntime{ctx: ctx}
					asOf := time.Now()
					calls := 0
					clk := evaluationClock{WithTicker: clock.RealClock{}, since: func(captured time.Time) time.Duration {
						require.Equal(t, asOf, captured, "must recheck the evaluated snapshot, not a refreshed timestamp")
						require.True(t, runtime.mu.TryLock(), "evaluation must not hold the lifetime lock")
						runtime.mu.Unlock()
						require.True(t, runtime.cache.mu.TryLock(), "evaluation must not hold the refresh lock")
						// A concurrent refresh cannot renew the captured decision.
						runtime.cache.asOf = asOf.Add(time.Second)
						runtime.cache.mu.Unlock()
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
					cache, err := NewPolicyCache(policySourceFunc(func(context.Context, int64) (*PolicySnapshot, error) {
						return snapshot, nil
					}), clk)
					require.NoError(t, err)
					cache.value, cache.asOf = snapshot, asOf
					runtime.cache = cache
					result, err := runtime.EvaluateRequest(testIdentity(), http.MethodGet, path)
					require.Equal(t, 2, calls)
					require.ErrorIs(t, err, ErrAuthorizationUnavailable)
					require.Equal(t, RequestEvaluation{Generation: -1}, result)
				})
			}
		}
	}
}

func TestProtectedMutationEncodedTargets(t *testing.T) {
	t.Parallel()
	for _, namespace := range []string{"Radius.Core", "rAdIuS.cOrE", "System.Authorization", "sYsTeM.aUtHoRiZaTiOn"} {
		for _, typ := range []string{"roleAssignments", "roleDefinitions", "authorizationSettings", "accessChanges"} {
			path := "/planes/azure/local/providers/" + namespace + "/" + typ + "/a"
			for depth := 0; depth <= 3; depth++ {
				require.True(t, protectedMutation(http.MethodPut, path), path)
				require.False(t, protectedMutation(http.MethodGet, path))
				path = url.PathEscape(path)
			}
		}
	}
	for _, path := range []string{
		"/planes/azure/local/providers/Radius%2eCore%2froleAssignments/a",
		"/planes/azure/local/providers/Radius%252eCore/role%2541ssignments/a",
		"/planes/azure/local/providers/Radius%25252eCore/roleAssignments/a",
		"/planes/azure/local/providers/System%252eAuthorization/policyObjects/a",
		"/providers/Radius.Core",
	} {
		require.Equal(t, path != "/providers/Radius.Core", protectedMutation(http.MethodPut, path), path)
	}
	for _, path := range []string{
		"/providers/Microsoft.Test/widgets/a",
		"/providers/Microsoft%2eTest/widgets/a%20b",
		"/providers/Microsoft%252eTest/widgets/a",
		"/providers/Microsoft%25252eTest/widgets/a",
	} {
		require.False(t, protectedMutation(http.MethodPut, path), path)
	}
	for _, path := range []string{"/providers/Microsoft.Test/widgets/%", "/providers/%zz/widgets/a", "/" + strings.Repeat("%25", 1) + strings.Repeat("25", 9)} {
		require.True(t, protectedMutation(http.MethodPut, path), path)
	}
}

type failingResponseWriter struct {
	header http.Header
	status int
}

func (w *failingResponseWriter) Header() http.Header    { return w.header }
func (w *failingResponseWriter) WriteHeader(status int) { w.status = status }
func (w *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("response disconnected")
}

func TestObserveRequestsErrorResponses(t *testing.T) {
	t.Parallel()
	var runtime *PolicyRuntime
	result, err := runtime.EvaluateRequest(testIdentity(), http.MethodGet, appA)
	require.ErrorIs(t, err, ErrAuthorizationUnavailable)
	require.Equal(t, RequestEvaluation{Generation: -1}, result)
	for _, authenticated := range []bool{false, true} {
		core, logs := observer.New(zap.InfoLevel)
		request := httptest.NewRequest(http.MethodPut, "/providers/Radius.Core/roleAssignments/a", nil)
		ctx := logr.NewContext(request.Context(), zapr.NewLogger(zap.New(core)))
		if authenticated {
			ctx = context.WithValue(ctx, identityContextKey{}, testIdentity())
		}
		writer := &failingResponseWriter{header: http.Header{}}
		ObserveRequests(nil, "")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("protected request reached handler")
		})).ServeHTTP(writer, request.WithContext(ctx))
		expectedStatus, expectedMessage := http.StatusUnauthorized, "Failed to write authentication error response"
		if authenticated {
			expectedStatus, expectedMessage = http.StatusServiceUnavailable, "Failed to write authorization error response"
		}
		require.Equal(t, expectedStatus, writer.status)
		require.Len(t, logs.FilterMessage(expectedMessage).All(), 1)
		require.Contains(t, logs.FilterMessage(expectedMessage).All()[0].ContextMap()["error"], "response disconnected")
	}
}

func TestObserveRequestsPathBaseDiscovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime, _, stop := requestRuntime(t, api.UserAuthorizationModeDryRun)
		defer stop()
		core, logs := observer.New(zap.InfoLevel)
		request := httptest.NewRequest(http.MethodGet, "/apis/api.ucp.dev/v1alpha3", nil)
		request = request.WithContext(logr.NewContext(request.Context(), zapr.NewLogger(zap.New(core))))
		response := httptest.NewRecorder()
		ObserveRequests(runtime, request.URL.Path)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/apis/api.ucp.dev/v1alpha3", r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(response, request)
		require.Equal(t, http.StatusNoContent, response.Code)
		require.Empty(t, logs.FilterMessage("User authorization request").All())
	})
}
