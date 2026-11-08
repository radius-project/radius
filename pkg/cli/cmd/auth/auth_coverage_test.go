// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestValidateMissingOutputFlag(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	commonflags.AddWorkspaceFlag(cmd)
	r := &runner{config: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)}}
	require.EqualError(t, r.Validate(cmd, nil), `flag accessed but not defined: output`)
	require.NotNil(t, r.workspace)
}

func TestValidateMissingWorkspaceFlag(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	commonflags.AddOutputFlag(cmd)
	r := &runner{config: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)}}
	require.EqualError(t, r.Validate(cmd, nil), `flag accessed but not defined: workspace`)
	require.Nil(t, r.workspace)
}

func TestCheckPrincipalIssuerFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("identity service unavailable")
	for _, test := range []struct {
		name     string
		identity api.WhoAmIResult
		err      error
		message  string
	}{
		{name: "identity request failed", err: failure, message: failure.Error()},
		{name: "missing verified issuer", message: "Radius returned no verified issuer"},
		{name: "empty verified issuer", identity: api.WhoAmIResult{Issuer: to.Ptr("")}, message: "Radius returned no verified issuer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			writer := &output.MockOutput{}
			factory := &framework.Impl{ConfigHolder: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)}, Output: writer}
			cmd, r := newCommand(factory, "check")
			client := &testClient{identity: &test.identity, err: test.err}
			r.connect = func(context.Context, workspaces.Workspace) (inspectionClient, error) { return client, nil }
			cmd.SetArgs([]string{"--principal", "group:developers", "--action", "Radius.Core/applications/read"})
			err := cmd.ExecuteContext(t.Context())
			require.EqualError(t, err, test.message)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			}
			require.Equal(t, 1, client.calls)
			require.Empty(t, client.request.Checks, "failed identity lookup must not send an access check")
			require.Empty(t, writer.Writes)
		})
	}
}

func TestRunRejectsInvalidPrincipal(t *testing.T) {
	t.Parallel()
	client := &testClient{}
	writer := &output.MockOutput{}
	r := &runner{
		operation: "check", workspace: &workspaces.Workspace{}, output: writer,
		principal: "user:", issuer: "kubernetes:test",
		connect: func(context.Context, workspaces.Workspace) (inspectionClient, error) { return client, nil },
	}
	// Cobra rejects this during validation; Run also rejects invalid direct callers.
	require.EqualError(t, r.Run(t.Context()), "principal must be user:<subject>, group:<subject>, or workload:<subject>")
	require.Zero(t, client.calls)
	require.Empty(t, client.request.Checks)
	require.Empty(t, writer.Writes)
}

func TestConnectInvalidWorkspace(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		connection map[string]any
		message    string
	}{
		{name: "missing kind", message: "workspace is missing required field"},
		{name: "unsupported kind", connection: map[string]any{"kind": "unsupported"}, message: "unsupported connection kind"},
		{name: "invalid endpoint", connection: map[string]any{"kind": "kubernetes", "overrides": map[string]any{"ucp": "://invalid"}}, message: "invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, err := connect(t.Context(), workspaces.Workspace{Connection: test.connection})
			require.ErrorContains(t, err, test.message)
			require.Nil(t, client)
		})
	}
}

func TestConnectUnhealthyEndpoint(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		status  int
		message string
	}{
		{status: http.StatusNotFound, message: "could not connect to radius"},
		{status: http.StatusForbidden, message: "Status Code: 403"},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/apis/api.ucp.dev/v1alpha3" {
					t.Errorf("unexpected health request: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(test.status)
			}))
			t.Cleanup(server.Close)
			client, err := connect(t.Context(), workspaces.Workspace{Connection: map[string]any{
				"kind": "kubernetes", "overrides": map[string]any{"ucp": server.URL},
			}})
			require.ErrorContains(t, err, test.message)
			require.Nil(t, client)
		})
	}
}

type inspectionHTTPRequest struct {
	method      string
	path        string
	version     string
	contentType string
	auth        string
	body        []byte
	err         error
}

func inspectionHTTPServer(t *testing.T, status int, response any) (*httptest.Server, <-chan inspectionHTTPRequest) {
	t.Helper()
	requests := make(chan inspectionHTTPRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/apis/api.ucp.dev/v1alpha3" {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, err := io.ReadAll(r.Body)
		requests <- inspectionHTTPRequest{
			method: r.Method, path: r.URL.Path, version: r.URL.Query().Get("api-version"),
			contentType: r.Header.Get("Content-Type"), auth: r.Header.Get("Authorization"), body: body, err: err,
		}
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("write inspection response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server, requests
}

func requireInspectionRequest(t *testing.T, requests <-chan inspectionHTTPRequest, method, path string) inspectionHTTPRequest {
	t.Helper()
	select {
	case request := <-requests:
		require.NoError(t, request.err)
		require.Equal(t, method, request.method)
		require.Equal(t, "/apis/api.ucp.dev/v1alpha3"+path, request.path)
		require.Equal(t, "2025-08-01-preview", request.version)
		require.Empty(t, request.auth, "anonymous SDK credential must not inject a bearer token")
		return request
	default:
		t.Fatal("inspection client did not send the expected HTTP request")
		return inspectionHTTPRequest{}
	}
}

func TestInspectionCommandsWithGeneratedHTTPClients(t *testing.T) {
	t.Parallel()
	for _, mode := range api.PossibleUserAuthorizationModeValues() {
		for _, format := range []string{"json", "table"} {
			for _, operation := range []string{"whoami", "status", "installation check", "scoped check"} {
				t.Run(fmt.Sprintf("%s/%s/%s", operation, mode, format), func(t *testing.T) {
					t.Parallel()
					identity, err := (&testClient{}).whoAmI(t.Context())
					require.NoError(t, err)
					identity.Mode = &mode
					response := any(identity)
					commandOperation, scope := operation, "/"
					isCheck := operation == "installation check" || operation == "scoped check"
					if isCheck {
						commandOperation = "check"
						if operation == "scoped check" {
							scope = "/planes/radius/local/resourceGroups/team-a"
						}
						response = api.CheckAccessResponse{
							Principal: &api.Principal{Type: to.Ptr(api.PrincipalTypeGroup), Subject: to.Ptr("developers"), Issuer: to.Ptr("kubernetes:test")},
							Mode:      &mode, Generation: to.Ptr(int64(19)),
							Checks: []*api.AccessCheckResult{
								{Permission: to.Ptr("Radius.Core/applications/read"), Target: &scope, Allowed: to.Ptr(true)},
								{Permission: to.Ptr("Radius.Core/applications/write"), Target: to.Ptr(scope + "/denied"), Allowed: to.Ptr(false)},
							},
						}
					}
					server, requests := inspectionHTTPServer(t, http.StatusOK, response)
					config := radcli.LoadConfigWithWorkspace(t)
					section, err := cli.ReadWorkspaceSection(config)
					require.NoError(t, err)
					workspace := section.Items[radcli.TestWorkspaceName]
					workspace.Connection["overrides"] = map[string]any{"ucp": server.URL}
					section.Items[radcli.TestWorkspaceName] = workspace
					cli.UpdateWorkspaceSection(config, section)
					var buffer bytes.Buffer
					cmd, _ := newCommand(&framework.Impl{
						ConfigHolder: &framework.ConfigHolder{Config: config},
						Output:       &output.OutputWriter{Writer: &buffer},
					}, commandOperation)
					args := []string{"--output", format}
					if isCheck {
						args = append(args, "--scope", scope, "--principal", "group:developers", "--issuer", "kubernetes:test",
							"--action", "Radius.Core/applications/read", "--target", scope,
							"--action", "Radius.Core/applications/write", "--target", scope+"/denied")
					}
					cmd.SetArgs(args)
					require.NoError(t, cmd.ExecuteContext(t.Context()))
					path, method := "/providers/Radius.Core/whoAmI", http.MethodGet
					if isCheck {
						method, path = http.MethodPost, "/providers/Radius.Core/checkAccess"
						if scope != "/" {
							path = scope + path
						}
					}
					request := requireInspectionRequest(t, requests, method, path)
					if isCheck {
						require.Equal(t, "application/json", request.contentType)
						var body api.AccessCheckRequest
						require.NoError(t, json.Unmarshal(request.body, &body))
						require.Equal(t, &api.Principal{Type: to.Ptr(api.PrincipalTypeGroup), Subject: to.Ptr("developers"), Issuer: to.Ptr("kubernetes:test")}, body.Principal)
						require.Equal(t, []*api.AccessCheck{
							{Permission: to.Ptr("Radius.Core/applications/read"), Target: &scope},
							{Permission: to.Ptr("Radius.Core/applications/write"), Target: to.Ptr(scope + "/denied")},
						}, body.Checks)
					} else {
						require.Empty(t, request.body)
					}
					require.Empty(t, requests, "explicit issuer must not cause an extra identity request")
					if format == "json" {
						if isCheck {
							var actual api.CheckAccessResponse
							require.NoError(t, json.Unmarshal(buffer.Bytes(), &actual))
							require.Equal(t, response, actual)
						} else {
							var actual api.WhoAmIResult
							require.NoError(t, json.Unmarshal(buffer.Bytes(), &actual))
							require.Equal(t, identity, actual)
						}
						return
					}
					if isCheck {
						for _, value := range []string{"SUBJECT", "MODE", "GENERATION", "PERMISSION", "TARGET", "ALLOWED",
							"developers", string(mode), "19", "Radius.Core/applications/read", "Radius.Core/applications/write", scope + "/denied", "true", "false"} {
							require.Contains(t, buffer.String(), value)
						}
					} else if operation == "whoami" {
						for _, value := range []string{"TYPE", "SUBJECT", "ISSUER", "GROUPS", "user", "alice", "kubernetes:test", "developers"} {
							require.Contains(t, buffer.String(), value)
						}
					} else {
						for _, value := range []string{"MODE", "RADIUS ADMINISTRATOR", "WARNINGS", string(mode), "false", *identity.Warnings[0]} {
							require.Contains(t, buffer.String(), value)
						}
					}
				})
			}
		}
	}
}

func TestGeneratedHTTPClientErrors(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"whoami", "installation check", "scoped check"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			server, requests := inspectionHTTPServer(t, http.StatusForbidden, map[string]any{
				"error": map[string]string{"code": "AuthorizationFailed", "message": "inspection permission denied"},
			})
			client, err := connect(t.Context(), workspaces.Workspace{Connection: map[string]any{
				"kind": "kubernetes", "overrides": map[string]any{"ucp": server.URL},
			}})
			require.NoError(t, err)
			path, method := "/providers/Radius.Core/whoAmI", http.MethodGet
			if operation == "whoami" {
				result, callErr := client.whoAmI(t.Context())
				err = callErr
				require.Equal(t, api.WhoAmIResult{}, result)
			} else {
				scope := "/"
				if operation == "scoped check" {
					scope = "/planes/radius/local"
				}
				result, callErr := client.checkAccess(t.Context(), scope, api.AccessCheckRequest{
					Checks: []*api.AccessCheck{{Permission: to.Ptr("Radius.Core/applications/read"), Target: &scope}},
				})
				err = callErr
				require.Equal(t, api.CheckAccessResponse{}, result)
				method, path = http.MethodPost, "/providers/Radius.Core/checkAccess"
				if scope != "/" {
					path = scope + path
				}
			}

			var responseError *azcore.ResponseError
			require.ErrorAs(t, err, &responseError)
			require.Equal(t, http.StatusForbidden, responseError.StatusCode)
			require.Equal(t, "AuthorizationFailed", responseError.ErrorCode)
			requireInspectionRequest(t, requests, method, path)
			require.Empty(t, requests, "authorization failures must not be retried")
		})
	}
}

func TestGeneratedWhoAmIMalformedResponse(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		response any
	}{
		{name: "groups must be an array", response: map[string]any{"groups": "developers"}},
		{name: "warnings must be an array", response: map[string]any{"warnings": "authorization disabled"}},
		{name: "identity must be an object", response: []string{"alice"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server, requests := inspectionHTTPServer(t, http.StatusOK, test.response)
			client, err := connect(t.Context(), workspaces.Workspace{Connection: map[string]any{
				"kind": "kubernetes", "overrides": map[string]any{"ucp": server.URL},
			}})
			require.NoError(t, err)
			result, err := client.whoAmI(t.Context())
			require.ErrorContains(t, err, "WhoAmIResult")
			require.ErrorContains(t, err, "cannot unmarshal")
			require.Equal(t, api.WhoAmIResult{}, result, "malformed identity must not be returned as a successful empty identity")
			request := requireInspectionRequest(t, requests, http.MethodGet, "/providers/Radius.Core/whoAmI")
			require.Empty(t, request.body)
			require.Empty(t, requests, "malformed responses must not be retried")
		})
	}
}
