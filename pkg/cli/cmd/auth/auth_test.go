// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

type testClient struct {
	err     error
	scope   string
	request api.AccessCheckRequest
	calls   int
}

func (c *testClient) whoAmI(context.Context) (api.WhoAmIResult, error) {
	c.calls++
	return api.WhoAmIResult{
		Principal: &api.Principal{Type: to.Ptr(api.PrincipalTypeUser), Issuer: to.Ptr("kubernetes:test"), Subject: to.Ptr("alice")},
		Issuer:    to.Ptr("kubernetes:test"), Mode: to.Ptr(api.UserAuthorizationModeDryRun),
		Groups:                []*api.Principal{{Type: to.Ptr(api.PrincipalTypeGroup), Issuer: to.Ptr("kubernetes:test"), Subject: to.Ptr("developers")}},
		IsRadiusAdministrator: to.Ptr(false), Warnings: []*string{to.Ptr("DryRun does not block requests.")},
	}, c.err
}

func (c *testClient) checkAccess(_ context.Context, scope string, request api.AccessCheckRequest) (api.CheckAccessResponse, error) {
	c.scope, c.request = scope, request
	result := api.CheckAccessResponse{
		Principal: request.Principal, Generation: to.Ptr(int64(7)), Mode: to.Ptr(api.UserAuthorizationModeDryRun),
	}
	if result.Principal == nil {
		result.Principal = &api.Principal{Type: to.Ptr(api.PrincipalTypeUser), Issuer: to.Ptr("kubernetes:test"), Subject: to.Ptr("alice")}
	}
	for i, check := range request.Checks {
		result.Checks = append(result.Checks, &api.AccessCheckResult{Permission: check.Permission, Target: check.Target, Allowed: to.Ptr(i%2 == 0)})
	}
	return result, c.err
}

func TestInspectionCommands(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"whoami", "status", "check"} {
		for _, format := range []string{"json", "table"} {
			t.Run(operation+"/"+format, func(t *testing.T) {
				var buffer bytes.Buffer
				factory := &framework.Impl{
					ConfigHolder: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)},
					Output:       &output.OutputWriter{Writer: &buffer},
				}
				cmd, runner := newCommand(factory, operation)
				client := &testClient{}
				runner.connect = func(context.Context, workspaces.Workspace) (inspectionClient, error) { return client, nil }
				args := []string{"--output", format}
				if operation == "check" {
					args = append(args, "--action", "Radius.Core/applications/read", "--action", "Radius.Core/applications/write")
				}
				cmd.SetArgs(args)
				require.NoError(t, cmd.ExecuteContext(t.Context()))
				if format == "json" {
					var wire map[string]any
					require.NoError(t, json.Unmarshal(buffer.Bytes(), &wire))
					require.Equal(t, "DryRun", wire["mode"])
					if operation == "check" {
						require.Equal(t, float64(7), wire["generation"])
						checks := wire["checks"].([]any)
						require.Len(t, checks, 2)
						require.Equal(t, true, checks[0].(map[string]any)["allowed"])
						require.Equal(t, false, checks[1].(map[string]any)["allowed"])
					} else {
						require.Equal(t, false, wire["isRadiusAdministrator"])
						require.Contains(t, wire, "groups")
						require.Contains(t, wire, "warnings")
					}
				} else {
					require.Contains(t, buffer.String(), map[string]string{"whoami": "developers", "status": "DryRun does not block requests.", "check": "false"}[operation])
				}
				if operation == "check" {
					require.Equal(t, runner.workspace.Scope, client.scope)
					for _, check := range client.request.Checks {
						require.Equal(t, client.scope, *check.Target)
					}
				}
			})
		}
	}
}

func TestCheckPrincipalAndBatchFlags(t *testing.T) {
	t.Parallel()
	for _, explicitIssuer := range []bool{false, true} {
		t.Run(map[bool]string{true: "explicit issuer", false: "verified issuer"}[explicitIssuer], func(t *testing.T) {
			factory := &framework.Impl{ConfigHolder: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)}, Output: &output.MockOutput{}}
			cmd, runner := newCommand(factory, "check")
			client := &testClient{}
			runner.connect = func(context.Context, workspaces.Workspace) (inspectionClient, error) { return client, nil }
			args := []string{"--scope", "/", "--principal", "group:system:authenticated",
				"--action", "Radius.Core/applications/read", "--target", "/planes/radius/local",
				"--action", "Radius.Core/applications/write", "--target", "/planes/radius/other"}
			if explicitIssuer {
				args = append(args, "--issuer", "kubernetes:test")
			}
			cmd.SetArgs(args)
			require.NoError(t, cmd.ExecuteContext(t.Context()))
			require.Equal(t, "/", client.scope)
			require.Equal(t, "system:authenticated", *client.request.Principal.Subject)
			require.Equal(t, "kubernetes:test", *client.request.Principal.Issuer)
			require.Equal(t, !explicitIssuer, client.calls == 1)
			require.Equal(t, "/planes/radius/other", *client.request.Checks[1].Target)
		})
	}
}

func TestInspectionCommandErrors(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{}, {"--action", ""}, {"unexpected"},
		{"--action", "x", "--scope", "relative"},
		{"--action", "x", "--target", "/", "--target", "/"},
		{"--action", "x", "--issuer", "kubernetes:test"},
		{"--action", "x", "--principal", "invalid:subject"},
		{"--action", "x", "--principal", "user:"},
		{"--action", "x", "--output", "invalid"},
	} {
		factory := &framework.Impl{ConfigHolder: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)}, Output: &output.MockOutput{}}
		cmd, runner := newCommand(factory, "check")
		runner.connect = func(context.Context, workspaces.Workspace) (inspectionClient, error) {
			t.Fatal("invalid command attempted connection")
			return nil, nil
		}
		cmd.SetArgs(args)
		require.Error(t, cmd.ExecuteContext(t.Context()), args)
	}
	for _, operation := range []string{"whoami", "status", "check"} {
		for _, connectionFailure := range []bool{false, true} {
			factory := &framework.Impl{ConfigHolder: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)}, Output: &output.MockOutput{}}
			cmd, runner := newCommand(factory, operation)
			failure := errors.New("AuthorizationUnavailable")
			runner.connect = func(context.Context, workspaces.Workspace) (inspectionClient, error) {
				if connectionFailure {
					return nil, failure
				}
				return &testClient{err: failure}, nil
			}
			if operation == "check" {
				cmd.SetArgs([]string{"--action", "Radius.Core/applications/read"})
			} else {
				cmd.SetArgs([]string{})
			}
			require.ErrorIs(t, cmd.ExecuteContext(t.Context()), failure)
			require.Empty(t, factory.Output.(*output.MockOutput).Writes)
		}
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestInspectionOutputFailure(t *testing.T) {
	factory := &framework.Impl{ConfigHolder: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)}, Output: &output.OutputWriter{Writer: failedWriter{}}}
	cmd, runner := newCommand(factory, "status")
	runner.connect = func(context.Context, workspaces.Workspace) (inspectionClient, error) { return &testClient{}, nil }
	cmd.SetArgs([]string{})
	require.ErrorIs(t, cmd.ExecuteContext(t.Context()), io.ErrClosedPipe)
}

func TestInspectionCommandTree(t *testing.T) {
	factory := &framework.Impl{ConfigHolder: &framework.ConfigHolder{Config: radcli.LoadConfigWithWorkspace(t)}, Output: &output.MockOutput{}}
	cmd := NewCommand(factory)
	for _, path := range [][]string{{"whoami"}, {"status"}, {"access", "check"}} {
		child, remaining, err := cmd.Find(path)
		require.NoError(t, err)
		require.Empty(t, remaining)
		require.NotNil(t, child.RunE)
		require.NotNil(t, child.Flags().Lookup("output"))
		require.NotNil(t, child.Flags().Lookup("workspace"))
	}
}
