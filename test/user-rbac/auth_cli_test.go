// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userrbac

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

func testAuthCLI(t *testing.T, cli *cliFixture) {
	var identity api.WhoAmIResult
	result := cli.success(t, "poc-admin", "auth", "whoami", "-o", "json")
	require.NoError(t, json.Unmarshal([]byte(result), &identity))
	require.True(t, *identity.IsRadiusAdministrator)
	require.Equal(t, "poc-admin", *identity.Principal.Subject)
	issuer := *identity.Issuer
	role := team + "/providers/Radius.Core/roleDefinitions/poc-auth-cli-reader"
	grant := cliApp + "/providers/Radius.Core/roleAssignments/poc-auth-cli-reader"

	t.Run("identity and permission discovery", func(t *testing.T) {
		require.Contains(t, cli.success(t, "poc-developer", "auth", "status"), "Enforce")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "permission", "list"), "Radius.Core/applications/read")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "permission", "show", "Radius.Core/applications/read"), "Radius.Core/applications")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "role", "list", "-o", "json"), "radius-administrator")
		out, err := cli.run(t, "poc-admin", "auth", "role", "update", "reader", "--scope", "/", "--description", "must not apply", "--yes")
		require.Error(t, err)
		require.Contains(t, out, "built-in")
	})
	t.Run("custom role and scoped grant lifecycle", func(t *testing.T) {
		args := []string{"auth", "role", "create", "poc-auth-cli-reader", "--scope", team,
			"--permission", "Radius.Core/applications/read", "--description", "CLI end-to-end reader", "--yes", "-o", "json"}
		out, err := cli.run(t, "poc-developer", args...)
		require.Error(t, err)
		require.Contains(t, out, "AuthorizationFailed")
		cli.success(t, "poc-admin", args...)
		t.Cleanup(func() {
			// Remove the role even if a later assertion fails, without hiding failures.
			out, err := cli.runContext(t, context.WithoutCancel(t.Context()), "poc-admin", "auth", "role", "delete", role, "--yes")
			if err != nil {
				t.Errorf("clean up custom role: %v: %s", err, out)
			}
		})
		require.Contains(t, cli.success(t, "poc-admin", "auth", "role", "show", role, "-o", "json"), "CLI end-to-end reader")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "role", "update", role,
			"--description", "Updated by the CLI", "--yes", "-o", "json"), "Updated by the CLI")
		cli.success(t, "poc-admin", "auth", "assignment", "create", "poc-auth-cli-reader", "--scope", cliApp,
			"--principal", "user:poc-outsider", "--issuer", issuer, "--role", "poc-auth-cli-reader", "--yes", "-o", "json")
		t.Cleanup(func() {
			// This cleanup is registered last so assignments are deleted before roles.
			out, err := cli.runContext(t, context.WithoutCancel(t.Context()), "poc-admin", "auth", "assignment", "delete", grant, "--yes")
			if err != nil && !strings.Contains(out, "NotFound") {
				t.Errorf("clean up custom assignment: %v: %s", err, out)
			}
		})
		cli.success(t, "poc-outsider", "app", "show", "cli-demo", "-o", "json")
		cli.denied(t, "poc-outsider", "Radius.Core/applications/write", "resource", "create",
			"Radius.Core/applications", "cli-demo", "-f", cli.dir+"/application.json")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "assignment", "update", grant,
			"--description", "Temporary access", "--display-name", "CLI outsider", "--yes", "-o", "json"), "Temporary access")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "assignment", "show", grant), "poc-outsider")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "assignment", "list", "--scope", cliApp,
			"--include-inherited", "--principal", "user:poc-outsider", "--issuer", issuer, "-o", "json"), "poc-auth-cli-reader")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "access", "list", "--scope", cliApp,
			"--principal", "user:poc-outsider", "--issuer", issuer, "-o", "json"), "Radius.Core/applications/read")
		var explanation api.ExplainAccessResponse
		out = cli.success(t, "poc-admin", "auth", "access", "explain", "--scope", cliApp,
			"--principal", "user:poc-outsider", "--issuer", issuer, "--action", "Radius.Core/applications/read", "-o", "json")
		require.NoError(t, json.Unmarshal([]byte(out), &explanation))
		require.True(t, *explanation.Checks[0].Allowed)
		require.Len(t, explanation.Checks[0].Grants, 1)
		cli.success(t, "poc-admin", "auth", "assignment", "delete", grant, "--yes")
		cli.denied(t, "poc-outsider", "Radius.Core/applications/read", "app", "show", "cli-demo")
		var check api.CheckAccessResponse
		out = cli.success(t, "poc-outsider", "auth", "access", "check", "--scope", cliApp,
			"--action", "Radius.Core/applications/read", "-o", "json")
		require.NoError(t, json.Unmarshal([]byte(out), &check))
		require.False(t, *check.Checks[0].Allowed)
	})
	t.Run("reader explanations redact grants", func(t *testing.T) {
		var explanation api.ExplainAccessResponse
		out := cli.success(t, "poc-reader", "auth", "access", "explain", "--scope", cliApp,
			"--action", "Radius.Core/applications/read", "-o", "json")
		require.NoError(t, json.Unmarshal([]byte(out), &explanation))
		require.True(t, *explanation.Checks[0].Allowed)
		require.Empty(t, explanation.Checks[0].Grants)
	})
	t.Run("history and administrator safeguards", func(t *testing.T) {
		require.Contains(t, cli.success(t, "poc-admin", "auth", "history", "list", "-o", "json"), "poc-auth-cli-reader")
		out, err := cli.run(t, "poc-admin", "auth", "assignment", "delete", "permanent-admins", "--scope", "/", "--yes")
		require.Error(t, err)
		require.Contains(t, out, "LastAdministratorProtected")
		out, err = cli.run(t, "poc-developer", "auth", "assignment", "create", "must-not-escalate",
			"--scope", "/", "--principal", "user:poc-developer", "--issuer", issuer,
			"--role", "/providers/Radius.Core/roleDefinitions/radius-administrator", "--yes")
		require.Error(t, err)
		require.Contains(t, out, "AuthorizationFailed")
	})
	t.Run("enforcement migration commands", func(t *testing.T) {
		require.Contains(t, cli.success(t, "poc-admin", "auth", "enforcement", "show", "-o", "json"), "Enforce")
		t.Cleanup(func() {
			out, err := cli.runContext(t, context.WithoutCancel(t.Context()), "poc-admin", "auth", "enforcement", "enable", "--yes")
			require.NoError(t, err, "%s", out)
		})
		require.Contains(t, cli.success(t, "poc-admin", "auth", "enforcement", "preview", "--yes", "-o", "json"), "DryRun")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "enforcement", "enable", "--yes", "-o", "json"), "Enforce")
		out, err := cli.run(t, "poc-admin", "auth", "enforcement", "set", "--mode", "Disabled", "--yes")
		require.Error(t, err)
		require.Contains(t, out, "returning to Disabled requires recovery")
	})
}
