// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

const testScope = "/planes/radius/local/resourceGroups/team"
const testApp = testScope + "/providers/Radius.Core/applications/demo"

func execute(t *testing.T, handler http.HandlerFunc, args ...string) (string, string, error) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	factory := &framework.Impl{
		ConfigHolder: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)},
		Output:       &output.OutputWriter{Writer: &stdout},
	}
	cmd := newCommand(factory, func(context.Context, *workspaces.Workspace) (sdk.Connection, error) {
		return sdk.NewDirectConnectionWithTransport(server.URL, server.Client().Transport)
	})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(t.Context())
	return stdout.String(), stderr.String(), err
}

func respond(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

func TestReadCommands(t *testing.T) {
	t.Parallel()
	for _, spec := range []struct {
		name string
		args []string
		path string
		body string
		want string
	}{
		{"identity", []string{"whoami"}, "/providers/Radius.Core/whoami", `{"issuer":"kubernetes:test","principal":{"type":"user","issuer":"kubernetes:test","subject":"alice"},"groups":[],"mode":"Enforce","isRadiusAdministrator":false}`, "alice"},
		{"status", []string{"status"}, "/providers/Radius.Core/whoami", `{"issuer":"kubernetes:test","principal":{"subject":"alice"},"mode":"Enforce","isRadiusAdministrator":false}`, "Enforce"},
		{"permissions", []string{"permission", "list"}, "/providers/Radius.Core/permissions", `{"value":[{"name":"Radius.Core/applications/read","description":"Read applications","displayName":"Read"}]}`, "Read applications"},
		{"permission explanation", []string{"permission", "explain", "Radius.Core/applications/read"}, "/providers/Radius.Core/permissions", `{"value":[{"name":"Radius.Core/applications/read","description":"Read applications","displayName":"Read"}]}`, "Read applications"},
		{"root roles", []string{"role", "list"}, "/providers/Radius.Core/roleDefinitions", `{"value":[{"name":"reader","properties":{"roleType":"BuiltIn","permissions":["Radius.Core/applications/read"]}}]}`, "reader"},
		{"scoped role", []string{"role", "show", "custom", "--scope", testScope}, testScope + "/providers/Radius.Core/roleDefinitions/custom", `{"name":"custom","properties":{"roleType":"Custom","permissions":["Radius.Core/applications/read"]}}`, "custom"},
		{"assignment ID", []string{"assignment", "show", testApp + "/providers/Radius.Core/roleAssignments/reader"}, testApp + "/providers/Radius.Core/roleAssignments/reader", `{"name":"reader","properties":{"principal":{"subject":"alice","issuer":"kubernetes:test"},"roleDefinitionId":"/providers/Radius.Core/roleDefinitions/reader","scope":"` + testApp + `"}}`, testApp},
		{"settings", []string{"enforcement", "show"}, "/providers/Radius.Core/authorizationSettings/default", `{"properties":{"mode":"DryRun","generation":7}}`, "DryRun"},
		{"history", []string{"history", "list"}, "/providers/Radius.Core/accessChanges", `{"value":[{"id":"change-7","properties":{"generation":7,"principal":{"subject":"alice"}}}]}`, "alice"},
		{"history detail", []string{"history", "show", "7"}, "/providers/Radius.Core/accessChanges/7", `{"id":"change-7","properties":{"generation":7,"principal":{"subject":"alice"}}}`, "alice"},
	} {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			for _, format := range []string{"table", "json"} {
				t.Run(format, func(t *testing.T) {
					out, _, err := execute(t, func(w http.ResponseWriter, req *http.Request) {
						require.Equal(t, http.MethodGet, req.Method)
						require.True(t, strings.EqualFold(spec.path, req.URL.Path), "expected path %s, got %s", spec.path, req.URL.Path)
						require.Equal(t, "2025-08-01-preview", req.URL.Query().Get("api-version"))
						w.Header().Set("Content-Type", "application/json")
						_, err := w.Write([]byte(spec.body))
						require.NoError(t, err)
					}, append(spec.args, "-o", format)...)
					require.NoError(t, err)
					require.Contains(t, out, spec.want)
					if format == "json" {
						require.True(t, json.Valid([]byte(out)), out)
					}
				})
			}
		})
	}
}

func TestPaginationAndInheritedAssignments(t *testing.T) {
	t.Parallel()
	calls := []string{}
	out, _, err := execute(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.URL.Path)
		role := map[string]any{"name": "page-one", "properties": map[string]any{"permissions": []string{"Radius.Core/applications/read"}}}
		if req.URL.Query().Get("page") == "2" {
			role["name"] = "page-two"
			respond(t, w, map[string]any{"value": []any{role}})
			return
		}
		respond(t, w, map[string]any{"value": []any{role}, "nextLink": "http://" + req.Host + req.URL.Path + "?api-version=2025-08-01-preview&page=2"})
	}, "role", "list", "-o", "json")
	require.NoError(t, err)
	require.Len(t, calls, 2)
	require.Contains(t, out, "page-one")
	require.Contains(t, out, "page-two")

	calls = nil
	out, _, err = execute(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.URL.Path)
		respond(t, w, map[string]any{"value": []any{}})
	}, "assignment", "list", "--scope", testApp, "--include-inherited", "-o", "json")
	require.NoError(t, err)
	require.JSONEq(t, "[]", out)
	require.Equal(t, []string{
		testApp + "/providers/Radius.Core/roleAssignments",
		testScope + "/providers/Radius.Core/roleAssignments",
		"/planes/radius/local/providers/Radius.Core/roleAssignments",
		"/providers/Radius.Core/roleAssignments",
	}, calls)
}

func TestAccessRequests(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"check", "explain", "list"} {
		t.Run(verb, func(t *testing.T) {
			t.Parallel()
			args := []string{"access", verb, "--scope", testApp, "--principal", "group:readers", "--issuer", "kubernetes:test", "-o", "json"}
			if verb != "list" {
				args = append(args, "--action", "Radius.Core/applications/read")
			}
			out, _, err := execute(t, func(w http.ResponseWriter, req *http.Request) {
				if verb == "list" {
					require.Equal(t, http.MethodGet, req.Method)
					require.Equal(t, testApp+"/providers/Radius.Core/effectiveAccess", req.URL.Path)
					require.JSONEq(t, `{"type":"group","subject":"readers","issuer":"kubernetes:test"}`, req.URL.Query().Get("principal"))
					respond(t, w, map[string]any{"scope": testApp, "permissions": []string{"Radius.Core/applications/read"}, "assignments": []any{}})
					return
				}
				require.Equal(t, http.MethodPost, req.Method)
				action := "checkAccess"
				if verb == "explain" {
					action = "explainAccess"
				}
				require.Equal(t, testApp+"/providers/Radius.Core/"+action, req.URL.Path)
				var body map[string]any
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				require.Equal(t, "group", body["principal"].(map[string]any)["type"])
				require.Len(t, body["checks"], 1)
				respond(t, w, map[string]any{"checks": []any{map[string]any{"permission": "Radius.Core/applications/read", "target": testApp, "allowed": true}}})
			}, args...)
			require.NoError(t, err)
			require.Contains(t, out, "Radius.Core/applications/read")
		})
	}
}

func TestMutationsAndConfirmation(t *testing.T) {
	t.Parallel()
	t.Run("assignment summary and server-owned fields", func(t *testing.T) {
		body := map[string]any{}
		out, summary, err := execute(t, func(w http.ResponseWriter, req *http.Request) {
			require.Equal(t, http.MethodPut, req.Method)
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			respond(t, w, body)
		}, "assignment", "create", "demo", "--principal", "user:alice", "--issuer", "kubernetes:test",
			"--role", "/providers/Radius.Core/roleDefinitions/reader", "--scope", testApp, "--yes", "-o", "json")
		require.NoError(t, err)
		require.True(t, json.Valid([]byte(out)))
		require.Contains(t, summary, "user:alice")
		require.Contains(t, summary, "kubernetes:test")
		require.Contains(t, summary, testApp)
		require.Contains(t, summary, "inherits to descendants")
		properties := body["properties"].(map[string]any)
		require.NotContains(t, properties, "scope")
		require.NotContains(t, properties, "managedBy")
	})
	t.Run("noninteractive requires yes", func(t *testing.T) {
		calls := 0
		_, _, err := execute(t, func(w http.ResponseWriter, req *http.Request) { calls++ },
			"assignment", "create", "demo", "--principal", "user:alice", "--issuer", "kubernetes:test",
			"--role", "reader", "--scope", testApp)
		require.ErrorContains(t, err, "--yes")
		require.Zero(t, calls)
	})
	t.Run("enforcement aliases", func(t *testing.T) {
		for _, verb := range []string{"preview", "enable"} {
			_, _, err := execute(t, func(w http.ResponseWriter, req *http.Request) {
				var body map[string]any
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				want := "DryRun"
				if verb == "enable" {
					want = "Enforce"
				}
				require.Equal(t, want, body["properties"].(map[string]any)["mode"])
				respond(t, w, body)
			}, "enforcement", verb, "--yes", "-o", "json")
			require.NoError(t, err)
		}
	})
}

func TestInvalidInputMakesNoRequests(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"access", "check", "--scope", testApp},
		{"role", "create", "custom", "--scope", testScope, "--yes"},
		{"role", "delete", "../admin", "--scope", testScope, "--yes"},
		{"role", "create", "custom", "--permission", "p", "--yes"},
		{"assignment", "create", "--principal", "group:readers", "--role", "reader", "--scope", testScope, "--yes"},
		{"assignment", "create", "--principal", "alien:alice", "--issuer", "kubernetes:test", "--role", "reader", "--scope", testScope, "--yes"},
		{"assignment", "list", "--scope", testScope + "?bad=true"},
		{"assignment", "delete", testApp + "/providers/Radius.Core/roleAssignments/name", "--scope", testScope, "--yes"},
		{"assignment", "update", "name", "--scope", testScope, "--principal", "user:other", "--issuer", "kubernetes:test", "--yes"},
		{"enforcement", "set", "--mode", "unrecognized", "--yes"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			_, _, err := execute(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid input reached the server") }, args...)
			require.Error(t, err)
		})
	}
}

func TestRoleNamesMustResolveUnambiguously(t *testing.T) {
	t.Parallel()
	for _, ambiguous := range []bool{false, true} {
		t.Run(fmt.Sprint(ambiguous), func(t *testing.T) {
			t.Parallel()
			writes := 0
			_, _, err := execute(t, func(w http.ResponseWriter, req *http.Request) {
				if req.Method == http.MethodGet {
					if req.URL.Path == "/providers/Radius.Core/roleDefinitions/reader" || ambiguous &&
						req.URL.Path == testScope+"/providers/Radius.Core/roleDefinitions/reader" {
						respond(t, w, map[string]any{"id": req.URL.Path, "properties": map[string]any{"permissions": []string{"Radius.Core/applications/read"}}})
						return
					}
					w.WriteHeader(http.StatusNotFound)
					respond(t, w, map[string]any{"error": map[string]string{"code": "NotFound"}})
					return
				}
				writes++
				var value map[string]any
				require.NoError(t, json.NewDecoder(req.Body).Decode(&value))
				require.Equal(t, "/providers/Radius.Core/roleDefinitions/reader", value["properties"].(map[string]any)["roleDefinitionId"])
				respond(t, w, value)
			}, "assignment", "create", "--principal", "user:alice", "--issuer", "kubernetes:test",
				"--role", "reader", "--scope", testApp, "--yes", "-o", "json")
			if ambiguous {
				require.ErrorContains(t, err, "ambiguous")
				require.Zero(t, writes)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, writes)
			}
		})
	}
}

func TestInheritedListingDoesNotHideAncestorDenial(t *testing.T) {
	t.Parallel()
	out, _, err := execute(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == testApp+"/providers/Radius.Core/roleAssignments" {
			respond(t, w, map[string]any{"value": []any{map[string]any{"id": testApp + "/providers/Radius.Core/roleAssignments/read"}}})
			return
		}
		w.WriteHeader(http.StatusForbidden)
		respond(t, w, map[string]any{"error": map[string]string{"code": "AuthorizationFailed"}})
	}, "assignment", "list", "--scope", testApp, "--include-inherited", "-o", "json")
	require.ErrorContains(t, err, "AuthorizationFailed")
	require.Empty(t, out)
}

func TestDenialsAreNotSuccessfulOutput(t *testing.T) {
	t.Parallel()
	out, _, err := execute(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		respond(t, w, map[string]any{"error": map[string]string{"code": "AuthorizationFailed", "message": "denied"}})
	}, "assignment", "list", "--scope", testScope, "-o", "json")
	require.ErrorContains(t, err, "AuthorizationFailed")
	require.Empty(t, out)
}
