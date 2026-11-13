// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userrbac

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

//go:embed templates/access.bicep
var authorizationBicep string

func testAuthorizationDeployments(t *testing.T, cli *cliFixture) {
	var who api.WhoAmIResult
	require.NoError(t, json.Unmarshal([]byte(cli.success(t, "poc-admin", "auth", "whoami", "-o", "json")), &who))
	issuer := *who.Issuer
	role := team + "/providers/Radius.Core/roleDefinitions/poc-template-reader"
	grant := func(scope string) string {
		id := uuid.NewSHA1(uuid.MustParse("11fb06fb-712d-4ddd-98c7-e71bbd588830"), []byte(strings.Join([]string{scope, strings.ToLower(role), issuer, "poc-outsider"}, "-")))
		return scope + "/providers/Radius.Core/roleAssignments/" + id.String()
	}
	file := filepath.Join(cli.dir, "access.bicep")
	require.NoError(t, os.WriteFile(file, []byte(authorizationBicep), 0600))
	parameters := []string{"--parameters", "identityIssuer=" + issuer, "--parameters", "assignmentScope=" + team}
	args := append([]string{"deploy", file}, parameters...)
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		for _, id := range []string{grant(team), grant(cliApp)} {
			out, err := cli.runContext(t, ctx, "poc-admin", "auth", "assignment", "delete", id, "--yes")
			if err != nil && !strings.Contains(out, "NotFound") {
				t.Errorf("clean up template assignment: %v: %s", err, out)
			}
		}
		out, err := cli.runContext(t, ctx, "poc-admin", "auth", "role", "delete", role, "--yes")
		if err != nil && !strings.Contains(out, "NotFound") {
			t.Errorf("clean up template role: %v: %s", err, out)
		}
	})
	t.Run("noninteractive confirmation before any changes", func(t *testing.T) {
		out, err := cli.run(t, "poc-admin", args...)
		require.Error(t, err)
		require.Contains(t, out, "--yes")
		out, err = cli.run(t, "poc-admin", "auth", "role", "show", role)
		require.Error(t, err)
		require.Contains(t, out, "NotFound")
	})
	t.Run("developer cannot deploy policy", func(t *testing.T) {
		cli.denied(t, "poc-developer", "Radius.Core/roleDefinitions/write", append(args, "--yes")...)
	})
	t.Run("resource-group policy without an environment", func(t *testing.T) {
		cli.success(t, "poc-admin", append(args, "--yes")...)
		require.Contains(t, cli.success(t, "poc-admin", "auth", "assignment", "show", grant(team), "-o", "json"), "Deployment")
		cli.success(t, "poc-outsider", "app", "show", "cli-demo")
	})
	t.Run("resource scope and incremental omission", func(t *testing.T) {
		scoped := []string{"deploy", file, "--parameters", "identityIssuer=" + issuer, "--parameters", "assignmentScope=" + cliApp, "--yes"}
		cli.success(t, "poc-admin", scoped...)
		cli.success(t, "poc-admin", "auth", "assignment", "show", grant(team))
		cli.success(t, "poc-admin", "auth", "assignment", "delete", grant(team), "--yes")
		cli.success(t, "poc-outsider", "app", "show", "cli-demo")
		cli.success(t, "poc-admin", append(scoped, "--parameters", "roleDescription=Updated by Bicep")...)
		require.Contains(t, cli.success(t, "poc-admin", "auth", "role", "show", role, "-o", "json"), "Updated by Bicep")
	})
	t.Run("invalid mixed template changes no resources", func(t *testing.T) {
		name := "poc-template-must-not-exist"
		mixed := authorizationBicep + fmt.Sprintf("\nresource application 'Radius.Core/applications@2025-08-01-preview' = {\n  name: '%s'\n  properties: {\n    environment: '%s'\n  }\n}\n", name, cliStaging)
		mixedFile := filepath.Join(cli.dir, "mixed-access.bicep")
		require.NoError(t, os.WriteFile(mixedFile, []byte(mixed), 0600))
		permissionsFile := filepath.Join(cli.dir, "invalid-permissions.json")
		writeJSON(t, permissionsFile, []string{"Radius.Core/unregistered/write"})
		out, err := cli.run(t, "poc-admin", "deploy", mixedFile, "--parameters", "identityIssuer="+issuer,
			"--parameters", "assignmentScope="+cliApp, "--parameters", "permissions=@"+permissionsFile, "--yes")
		require.Error(t, err)
		require.Contains(t, out, "UnknownPermission")
		out, err = cli.run(t, "poc-admin", "app", "show", name)
		require.Error(t, err)
		require.Contains(t, out, "not found")
		require.Contains(t, cli.success(t, "poc-admin", "auth", "role", "show", role, "-o", "json"), "Updated by Bicep")
	})
	t.Run("valid mixed policy and workload template", func(t *testing.T) {
		name := "poc-template-workload"
		mixed := authorizationBicep + fmt.Sprintf("\nresource application 'Radius.Core/applications@2025-08-01-preview' = {\n  name: '%s'\n  properties: {\n    environment: '%s'\n  }\n}\n", name, cliStaging)
		mixedFile := filepath.Join(cli.dir, "mixed-access.bicep")
		require.NoError(t, os.WriteFile(mixedFile, []byte(mixed), 0600))
		t.Cleanup(func() {
			out, err := cli.runContext(t, context.WithoutCancel(t.Context()), "poc-admin", "resource", "delete", "Radius.Core/applications", name, "--yes")
			if err != nil && !strings.Contains(out, "NotFound") {
				t.Errorf("clean up mixed-template application: %v: %s", err, out)
			}
		})
		cli.success(t, "poc-admin", "deploy", mixedFile, "--parameters", "identityIssuer="+issuer,
			"--parameters", "assignmentScope="+cliApp, "--yes")
		cli.success(t, "poc-admin", "app", "show", name)
		cli.denied(t, "poc-outsider", "Radius.Core/applications/read", "app", "show", name)
	})
}
