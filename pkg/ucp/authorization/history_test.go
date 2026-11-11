// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const historyPath = "/providers/Radius.Core/accessChanges"

func TestManagementHistoryEveryMode(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				policy := initialPolicy()
				policy.Settings.Properties.Mode = &mode
				for _, grant := range []struct{ name, role, scope string }{
					{"auditor", "auditor", "/"},
					{"scoped", "auditor", teamA},
					{"reader", "reader", "/"},
				} {
					assignment := testAssignment(grant.name, builtInID(grant.role), grant.scope,
						testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", grant.name))
					assignment.Properties.ManagedBy = &api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)}
					assignment.Properties.CreatedBy = to.Ptr("trusted-startup")
					policy.Assignments = append(policy.Assignments, assignment)
				}
				store, runtime := managementRuntime(t, inmemory.NewClient(), policy)
				const pathBase = "/apis/api.ucp.dev/v1alpha3"
				handler := managementHandler(runtime, pathBase)
				admin := policyActor().Identity
				empty := managementRequest(t, handler, &admin, "GET", pathBase+historyPath+managementVersion, "")
				require.Equal(t, http.StatusOK, empty.Code, empty.Body.String())
				require.JSONEq(t, `{"value":[]}`, empty.Body.String())
				for _, mutation := range []PolicyMutation{
					readerMutation(),
					assignmentMutation("admin", builtInID("access-administrator"), teamB, "charlie"),
					assignmentMutation("self", builtInID("reader"), teamB, *admin.Principal.Subject),
				} {
					require.NoError(t, runtime.Mutate(t.Context(), policyActor(), mutation))
				}
				for _, subject := range []string{"alice", "auditor", "scoped", "reader", "nobody", ""} {
					identity := Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", subject)}
					var caller *Identity = &identity
					expected := http.StatusForbidden
					if subject == "alice" || subject == "auditor" {
						expected = http.StatusOK
					}
					if subject == "" {
						caller, expected = nil, http.StatusUnauthorized
					}
					for _, suffix := range []string{"", "/2", "/999"} {
						response := managementRequest(t, handler, caller, "GET", pathBase+historyPath+suffix+managementVersion, "")
						want := expected
						if expected == http.StatusOK && suffix == "/999" {
							want = http.StatusNotFound
						}
						require.Equal(t, want, response.Code, "%s %s: %s", subject, suffix, response.Body.String())
					}
				}
				event, err := store.AccessChange(t.Context(), 1)
				require.NoError(t, err)
				actorJSON, err := json.Marshal(admin.Principal)
				require.NoError(t, err)
				for _, test := range []struct {
					query string
					count int
				}{
					{"", 3},
					{"&scope=" + url.QueryEscape(strings.ToUpper(teamA)), 1},
					{"&scope=" + url.QueryEscape(teamB), 2},
					{"&scope=%2F", 3},
					{"&principal=" + url.QueryEscape(string(actorJSON)), 3},
					{"&principal=" + url.QueryEscape(`{"type":"user","issuer":"kubernetes:cluster-a","subject":"bob"}`), 0},
					{"&since=" + url.QueryEscape(event.Properties.Time.Format(time.RFC3339Nano)), 3},
					{"&since=" + url.QueryEscape(event.Properties.Time.Add(time.Second).Format(time.RFC3339Nano)), 0},
				} {
					response := managementRequest(t, handler, &admin, "GET", pathBase+historyPath+managementVersion+test.query, "")
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					var page api.AuthorizationListAccessChangeResource
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
					require.NotNil(t, page.Value)
					require.Len(t, page.Value, test.count, test.query)
					require.Nil(t, page.NextLink)
					for _, value := range page.Value {
						require.Equal(t, "Radius.Core/accessChanges", *value.Type)
						require.Equal(t, fmt.Sprint(*value.Properties.Generation), *value.Name)
						require.Equal(t, mode, *value.Properties.Mode)
						require.Equal(t, admin.Principal, *value.Properties.Principal)
						require.Equal(t, *value.Properties.Generation != 1, *value.Properties.HighImpact)
					}
				}
			})
		})
	}
}

func TestManagementHistoryPaging(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, runtime := managementRuntime(t, inmemory.NewClient(), initialPolicy())
		actor := policyActor()
		roleID := "/providers/Radius.Core/roleDefinitions/changing"
		mutation := PolicyMutation{Target: roleID, Role: customRole(roleID, "Radius.Core/applications/read").Properties}
		for i := range 103 {
			mutation.Role.Description = to.Ptr(fmt.Sprint(i))
			require.NoError(t, runtime.Mutate(t.Context(), actor, mutation))
		}
		const pathBase = "/apis/api.ucp.dev/v1alpha3"
		handler := managementHandler(runtime, pathBase)
		path := pathBase + historyPath + managementVersion
		response := managementRequest(t, handler, &actor.Identity, "GET", path, "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var first api.AuthorizationListAccessChangeResource
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &first))
		require.Len(t, first.Value, 100)
		require.Equal(t, int64(1), *first.Value[0].Properties.Generation)
		require.Equal(t, int64(100), *first.Value[99].Properties.Generation)
		require.NotNil(t, first.NextLink)
		require.True(t, strings.HasPrefix(*first.NextLink, pathBase+historyPath+"?"))
		mutation.Role.Description = to.Ptr("new generation after page one")
		require.NoError(t, runtime.Mutate(t.Context(), actor, mutation))
		response = managementRequest(t, handler, &actor.Identity, "GET", *first.NextLink, "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var second api.AuthorizationListAccessChangeResource
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &second))
		require.Len(t, second.Value, 3, "continuation does not pick up a newer generation")
		require.Equal(t, int64(103), *second.Value[2].Properties.Generation)
		require.Nil(t, second.NextLink)

		// Limit scanned records, not matching results. An empty filtered page
		// must still continue, rather than scan an unbounded history.
		response = managementRequest(t, handler, &actor.Identity, "GET", path+"&scope="+url.QueryEscape(teamA), "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &first))
		require.Empty(t, first.Value)
		require.NotNil(t, first.NextLink)
		require.Contains(t, *first.NextLink, "scope=")
		response = managementRequest(t, handler, &actor.Identity, "GET", *first.NextLink, "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		second = api.AuthorizationListAccessChangeResource{}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &second))
		require.Empty(t, second.Value)
		require.Nil(t, second.NextLink)

		require.NoError(t, runtime.Mutate(t.Context(), actor, PolicyMutation{
			Target: "/providers/Radius.Core/roleAssignments/alpha", Delete: true,
		}))
		revoked := actor.Identity
		revoked.Groups = nil
		response = managementRequest(t, handler, &revoked, "GET", *first.NextLink, "")
		require.Equal(t, http.StatusForbidden, response.Code, "continuation rechecks read access")
		require.NotContains(t, response.Body.String(), "policyChange")
	})
}

func TestManagementHistoryChunkedBytePaging(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, runtime := managementRuntime(t, inmemory.NewClient(), initialPolicy())
		actor := policyActor()
		mutation := largeHistoryMutation()
		for i := range 12 {
			mutation.Assignment.Description = to.Ptr(fmt.Sprint(i) + strings.Repeat("x", 130400))
			require.NoError(t, runtime.Mutate(t.Context(), actor, mutation))
		}
		handler := managementHandler(runtime, "")
		path := historyPath + managementVersion
		generation, pages := int64(1), 0
		for path != "" {
			response := managementRequest(t, handler, &actor.Identity, "GET", path, "")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Less(t, response.Body.Len(), historyPageBytes+1024)
			var page api.AuthorizationListAccessChangeResource
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
			require.NotEmpty(t, page.Value)
			for _, event := range page.Value {
				require.Equal(t, generation, *event.Properties.Generation)
				require.Greater(t, len(*event.Properties.PolicyChange.After.RoleAssignment.Description), 130000)
				generation++
			}
			path = ""
			if page.NextLink != nil {
				path = *page.NextLink
			}
			pages++
			require.Less(t, pages, 12, "continuation must make progress")
		}
		require.Equal(t, int64(13), generation)
		require.Greater(t, pages, 1)
	})
}

func TestPolicyStoreCommittedChangeLog(t *testing.T) {
	for _, recovering := range []bool{false, true} {
		t.Run(fmt.Sprint(recovering), func(t *testing.T) {
			db := &policyHookClient{Client: inmemory.NewClient()}
			store := newTestPolicyStore(t, db, true)
			core, logs := observer.New(zap.InfoLevel)
			ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))
			crash := errors.New("stop after claiming intent")
			db.after = func(_ context.Context, object *database.Object) error {
				if recovering && saveStage(object) == "claim" {
					return crash
				}
				return nil
			}
			mutation := assignmentMutation("new-admin", builtInID("access-administrator"), teamA, "bob")
			_, err := store.Mutate(ctx, policyActor(), mutation)
			if recovering {
				require.ErrorIs(t, err, crash)
				require.Empty(t, logs.All(), "preparation is not a committed access change")
				_, err = store.LoadPolicy(ctx, -1)
			}
			require.NoError(t, err)
			_, err = store.Mutate(ctx, policyActor(), mutation)
			require.NoError(t, err, "identical writes do not log another change")
			entries := logs.FilterMessage("User authorization policy change").All()
			require.Len(t, entries, 1)
			require.Equal(t, "radius.audit", entries[0].LoggerName)
			require.EqualValues(t, 1, entries[0].ContextMap()["generation"])
			raw, err := json.Marshal(entries[0].ContextMap()["accessChange"])
			require.NoError(t, err)
			var properties api.AccessChangeProperties
			require.NoError(t, json.Unmarshal(raw, &properties))
			require.True(t, *properties.HighImpact)
			require.Equal(t, policyActor().Identity.Principal, *properties.Principal)
			require.Equal(t, "request-1", *properties.RequestID)
			require.Equal(t, "correlation-1", *properties.CorrelationID)
			require.Equal(t, strings.ToLower(mutation.Target), *properties.PolicyChange.Target)
			require.Equal(t, api.AssignmentManagementKindImperative, *properties.PolicyChange.ManagedBy.Kind)
		})
	}
}

func TestManagementHistoryInvalidInputs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, runtime := managementRuntime(t, inmemory.NewClient(), initialPolicy())
		admin := policyActor().Identity
		handler := managementHandler(runtime, "")
		for _, path := range []string{
			historyPath,
			historyPath + managementVersion + "&api-version=2025-08-01-preview",
			historyPath + managementVersion + "&since=bad",
			historyPath + managementVersion + "&since=2026-01-01",
			historyPath + managementVersion + "&scope=invalid",
			historyPath + managementVersion + "&scope=%2F&scope=%2F",
			historyPath + managementVersion + "&principal=null",
			historyPath + managementVersion + "&principal=" + url.QueryEscape(`{"type":"user","issuer":"kubernetes:cluster-a","subject":"alice","groups":[]}`),
			historyPath + managementVersion + "&principal=" + url.QueryEscape(`{"type":"user","issuer":"kubernetes:cluster-a","subject":"alice"} {}`),
			historyPath + managementVersion + "&$skiptoken=1",
			historyPath + managementVersion + "&$skiptoken=2:1",
			historyPath + managementVersion + "&$skiptoken=1:2",
			historyPath + managementVersion + "&$skiptoken=01:2",
			historyPath + managementVersion + "&$skiptoken=1:9007199254740992",
			historyPath + managementVersion + "&unknown=true",
			historyPath + "/0" + managementVersion,
			historyPath + "/01" + managementVersion,
			historyPath + "/not-a-generation" + managementVersion,
			historyPath + "/9007199254740992" + managementVersion,
			historyPath + "/1" + managementVersion + "&scope=%2F",
			teamA + historyPath + managementVersion,
		} {
			response := managementRequest(t, handler, &admin, "GET", path, "")
			require.Equal(t, http.StatusBadRequest, response.Code, "%s: %s", path, response.Body.String())
		}
	})
}

func TestManagementHistoryUnavailable(t *testing.T) {
	for _, scenario := range []string{"missing runtime", "stale", "storage", "expires during read", "corrupt generation", "uncommitted"} {
		t.Run(scenario, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				db := &runtimeDatabase{Client: inmemory.NewClient()}
				store, runtime := managementRuntime(t, db, initialPolicy())
				actor := policyActor()
				require.NoError(t, runtime.Mutate(t.Context(), actor, readerMutation()))
				expected := http.StatusServiceUnavailable
				switch scenario {
				case "missing runtime":
					runtime = nil
				case "stale":
					runtime.cache.asOf = runtime.cache.asOf.Add(-PolicyMaxAge)
				case "storage":
					db.setGet(func(_ context.Context, id string) error {
						if strings.HasPrefix(id, historyPrefix) {
							return errors.New("private database failure")
						}
						return nil
					})
				case "expires during read":
					db.setGet(func(_ context.Context, id string) error {
						if strings.HasPrefix(id, historyPrefix) {
							time.Sleep(PolicyMaxAge)
						}
						return nil
					})
				case "corrupt generation":
					event, err := store.AccessChange(t.Context(), 1)
					require.NoError(t, err)
					event.ID, event.Properties.Generation = to.Ptr(historyPrefix+"2"), to.Ptr(int64(2))
					require.NoError(t, store.save(t.Context(), historyPrefix+"1", event))
				case "uncommitted":
					// Private preparations do not become visible merely because
					// they have a well-formed event or an existing public ID.
					event, err := store.AccessChange(t.Context(), 1)
					require.NoError(t, err)
					event.ID, event.Properties.Generation = to.Ptr(historyPrefix+"2"), to.Ptr(int64(2))
					require.NoError(t, store.save(t.Context(), historyPrefix+"2", event))
					expected = http.StatusNotFound
				}
				path := historyPath + "/1"
				if scenario == "uncommitted" {
					path = historyPath + "/2"
				}
				response := managementRequest(t, managementHandler(runtime, ""), &actor.Identity, "GET", path+managementVersion, "")
				require.Equal(t, expected, response.Code, response.Body.String())
				require.NotContains(t, response.Body.String(), "private")
				require.NotContains(t, response.Body.String(), "policyChange")
			})
		})
	}
}
