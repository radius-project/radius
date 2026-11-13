// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userrbac

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const cliStaging = platform + "/providers/Radius.Core/environments/cli-staging"
const cliProduction = platform + "/providers/Radius.Core/environments/cli-production"
const cliApp = team + "/providers/Radius.Core/applications/cli-demo"

//go:embed templates/application.bicep
var applicationBicep string

type cliFixture struct {
	binary string
	dir    string
	home   string
	config string
	log    *os.File
}

func writeJSON(t *testing.T, filename string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filename, raw, 0600))
}

func prepareCLI(t *testing.T, kubeconfig, contextName string) *cliFixture {
	t.Helper()
	binary := os.Getenv("RADIUS_USER_RBAC_RAD")
	require.NotEmpty(t, binary, "set RADIUS_USER_RBAC_RAD to this checkout's built rad binary; the CLI scenarios must not be skipped")
	binary, err := filepath.Abs(binary)
	require.NoError(t, err)
	info, err := os.Stat(binary)
	require.NoError(t, err)
	require.False(t, info.IsDir())
	dir := filepath.Join(filepath.Dir(kubeconfig), "cli")
	home := filepath.Join(dir, "home")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".kube"), 0700))
	config, err := clientcmd.LoadFromFile(kubeconfig)
	require.NoError(t, err)
	config.CurrentContext = contextName
	require.NoError(t, clientcmdapi.MinifyConfig(config))
	require.NoError(t, clientcmdapi.FlattenConfig(config))
	source := config.Contexts[contextName]
	auth := config.AuthInfos[source.AuthInfo]
	config.Contexts = map[string]*clientcmdapi.Context{}
	config.AuthInfos = map[string]*clientcmdapi.AuthInfo{}
	section := cli.WorkspaceSection{Default: "poc-outsider", Items: map[string]workspaces.Workspace{}}
	for _, identity := range []struct {
		name, group string
	}{
		{"poc-admin", "poc-admins"},
		{"poc-developer", "poc-developers"},
		{"poc-reader", "poc-readers"},
		{"poc-outsider", ""},
	} {
		name := contextName + "-" + identity.name
		context := source.DeepCopy()
		context.AuthInfo = identity.name
		config.Contexts[name] = context
		user := auth.DeepCopy()
		user.Impersonate = identity.name
		user.ImpersonateUID = ""
		user.ImpersonateUserExtra = nil
		user.ImpersonateGroups = []string{"system:authenticated"}
		if identity.group != "" {
			user.ImpersonateGroups = append(user.ImpersonateGroups, identity.group)
		}
		config.AuthInfos[identity.name] = user
		section.Items[identity.name] = workspaces.Workspace{
			Connection:  map[string]any{"kind": "kubernetes", "context": name},
			Scope:       team,
			Environment: cliStaging,
		}
	}
	// Selecting a workspace must override this unprivileged default context.
	config.CurrentContext = contextName + "-poc-outsider"
	require.NoError(t, clientcmd.WriteToFile(*config, filepath.Join(home, ".kube", "config")))
	configPath := filepath.Join(dir, "rad.json")
	writeJSON(t, configPath, map[string]any{"workspaces": section})
	extension := os.Getenv("RADIUS_USER_RBAC_BICEP_EXTENSION")
	require.NotEmpty(t, extension, "run the POC test wrapper to build this checkout's authorization Bicep extension")
	require.FileExists(t, extension)
	writeJSON(t, filepath.Join(dir, "bicepconfig.json"), map[string]any{"extensions": map[string]string{"radius": extension}})
	log, err := os.OpenFile(filepath.Join(dir, "commands.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	return &cliFixture{binary: binary, dir: dir, home: home, config: configPath, log: log}
}

func (c *cliFixture) run(t *testing.T, identity string, args ...string) (string, error) {
	t.Helper()
	return c.runContext(t, t.Context(), identity, args...)
}

func (c *cliFixture) runContext(t *testing.T, parent context.Context, identity string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	if identity != "" {
		args = append(args, "--workspace", identity)
	}
	args = append(args, "--config", c.config)
	command := exec.CommandContext(ctx, c.binary, args...)
	command.Dir = c.dir
	command.Env = append(os.Environ(), "HOME="+c.home,
		"KUBECONFIG="+filepath.Join(c.home, ".kube", "config"), "RADIUS_PREVIEW=false")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	_, logErr := fmt.Fprintf(c.log, "$ rad %s\nstdout:\n%sstderr:\n%s\nresult: %v\n",
		strings.Join(args, " "), stdout.String(), stderr.String(), err)
	require.NoError(t, logErr)
	t.Logf("rad %s -> %v\n%s%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	require.NoError(t, ctx.Err(), "CLI command timed out")
	if err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit, "failed to start the built CLI")
		return stdout.String() + stderr.String(), err
	}
	return stdout.String(), nil
}

func (c *cliFixture) success(t *testing.T, identity string, args ...string) string {
	t.Helper()
	output, err := c.run(t, identity, args...)
	require.NoError(t, err, "%s", output)
	return output
}

func (c *cliFixture) denied(t *testing.T, identity, permission string, args ...string) {
	t.Helper()
	output, err := c.run(t, identity, args...)
	require.Error(t, err, "%s", output)
	require.Contains(t, output, "AuthorizationFailed")
	require.Contains(t, strings.ToLower(output), strings.ToLower(permission))
}

func testCLI(t *testing.T, cli *cliFixture, kube kubernetes.Interface, admin apiClient, assignment func(string, string, string) map[string]any) {
	for _, fixture := range []struct{ name, namespace string }{
		{"cli-staging", "rbac-poc-cli-staging"}, {"cli-production", "rbac-poc-cli-production"},
	} {
		_, err := kube.CoreV1().Namespaces().Apply(t.Context(), corev1.Namespace(fixture.namespace), metav1.ApplyOptions{FieldManager: "radius-user-rbac-poc"})
		require.NoError(t, err)
		cli.success(t, "poc-admin", "env", "create", fixture.name, "--group", "rbac-poc-platform",
			"--kubernetes-namespace", fixture.namespace)
	}
	for _, role := range []struct {
		name, scope string
		permissions []string
	}{
		{"poc-cli-developer", team, []string{"Radius.Core/applications/read", "Radius.Core/applications/write",
			"Radius.Core/applications/delete", "Microsoft.Resources/deployments/read", "Microsoft.Resources/deployments/write"}},
		{"poc-cli-environment-reader", platform, []string{"Radius.Core/environments/read"}},
		{"poc-cli-discovery", "/planes/radius/local", []string{"System.Resources/providers/read"}},
	} {
		admin.request(http.MethodPut, "/providers/Radius.Core/roleDefinitions/"+role.name,
			map[string]any{"properties": map[string]any{
				"permissions": role.permissions, "assignableScopes": []string{role.scope},
			}}, http.StatusOK)
	}
	for _, grant := range []struct{ scope, name, role, group string }{
		{team, "cli-developers", "poc-cli-developer", "poc-developers"},
		{platform, "cli-environment-readers", "poc-cli-environment-reader", "poc-developers"},
		{"/planes/radius/local", "cli-discovery", "poc-cli-discovery", "system:authenticated"},
		{cliStaging, "cli-staging-deployers", "environment-deployer", "poc-developers"},
	} {
		admin.request(http.MethodPut, grant.scope+"/providers/Radius.Core/roleAssignments/"+grant.name,
			assignment(grant.role, "group", grant.group), http.StatusOK)
	}
	stagingFile := filepath.Join(cli.dir, "application.bicep")
	require.NoError(t, os.WriteFile(stagingFile, []byte(applicationBicep), 0600))
	updateFile := filepath.Join(cli.dir, "application.json")
	writeJSON(t, updateFile, map[string]any{
		"location": "global", "properties": map[string]string{"environment": cliStaging},
		"tags": map[string]string{"changed": "must-not-apply"},
	})

	if !t.Run("developer deploys using workspace defaults", func(t *testing.T) {
		result := cli.success(t, "poc-developer", "deploy", stagingFile)
		require.Contains(t, result, "Deployment Complete")
		admin.forTest(t).request(http.MethodPut, cliApp+"/providers/Radius.Core/roleAssignments/cli-readers",
			assignment("reader", "group", "poc-readers"), http.StatusOK)
		result = cli.success(t, "poc-reader", "app", "show", "cli-demo", "-o", "json")
		var application struct {
			ID         string `json:"id"`
			Properties struct {
				Environment       string `json:"environment"`
				ProvisioningState string `json:"provisioningState"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal([]byte(result), &application))
		require.True(t, strings.EqualFold(cliApp, application.ID), "unexpected application ID: %s", application.ID)
		require.True(t, strings.EqualFold(cliStaging, application.Properties.Environment), "unexpected environment: %s", application.Properties.Environment)
		require.Equal(t, "Succeeded", application.Properties.ProvisioningState)
	}) {
		return
	}
	t.Run("reader lists applications and outsider sees none", func(t *testing.T) {
		for _, identity := range []string{"poc-reader", "poc-outsider"} {
			var applications []struct {
				Name string `json:"name"`
			}
			result := cli.success(t, identity, "app", "list", "-o", "json")
			require.NoError(t, json.Unmarshal([]byte(result), &applications))
			if identity == "poc-outsider" {
				require.Empty(t, applications)
			} else {
				names := make([]string, 0, len(applications))
				for _, application := range applications {
					names = append(names, application.Name)
				}
				require.Contains(t, names, "cli-demo")
			}
		}
	})
	t.Run("reader writes and outsider reads are denied", func(t *testing.T) {
		cli.denied(t, "poc-reader", "Radius.Core/applications/write",
			"resource", "create", "Radius.Core/applications", "cli-demo", "-f", updateFile)
		cli.denied(t, "poc-reader", "Radius.Core/applications/delete",
			"resource", "delete", "Radius.Core/applications", "cli-demo", "--yes")
		for _, identity := range []string{"poc-outsider", ""} {
			cli.denied(t, identity, "Radius.Core/applications/read", "app", "show", "cli-demo")
		}
		result := admin.forTest(t).request(http.MethodGet, cliApp, nil, http.StatusOK)
		require.NotContains(t, string(result), "must-not-apply")
	})
	t.Run("production override is denied before mutation", func(t *testing.T) {
		cli.denied(t, "poc-developer", "Radius.Core/environments/use/action",
			"deploy", stagingFile, "--parameters", "applicationName=cli-must-not-exist",
			"--environment", cliProduction, "--group", "rbac-poc-team")
		admin.forTest(t).request(http.MethodGet, team+"/providers/Radius.Core/applications/cli-must-not-exist", nil, http.StatusNotFound)
	})
	t.Run("group override does not inherit the workspace grant", func(t *testing.T) {
		cli.denied(t, "poc-developer", "Radius.Core/applications/read at "+platform,
			"app", "show", "cli-demo", "--group", "rbac-poc-platform")
	})
	t.Run("staging use revocation reaches the CLI", func(t *testing.T) {
		admin.forTest(t).request(http.MethodDelete, cliStaging+"/providers/Radius.Core/roleAssignments/cli-staging-deployers", nil, http.StatusNoContent)
		cli.denied(t, "poc-developer", "Radius.Core/environments/use/action", "deploy", stagingFile)
		cli.success(t, "poc-developer", "app", "show", "cli-demo", "-o", "json")
	})
	t.Run("reader revocation reaches the CLI", func(t *testing.T) {
		admin.forTest(t).request(http.MethodDelete, cliApp+"/providers/Radius.Core/roleAssignments/cli-readers", nil, http.StatusNoContent)
		cli.denied(t, "poc-reader", "Radius.Core/applications/read", "app", "show", "cli-demo")
		cli.success(t, "poc-admin", "app", "show", "cli-demo", "-o", "json")
	})
	t.Run("restoring CLI grants for manual testing", func(t *testing.T) {
		admin.forTest(t).request(http.MethodPut, cliApp+"/providers/Radius.Core/roleAssignments/cli-readers",
			assignment("reader", "group", "poc-readers"), http.StatusOK)
		admin.forTest(t).request(http.MethodPut, cliStaging+"/providers/Radius.Core/roleAssignments/cli-staging-deployers",
			assignment("environment-deployer", "group", "poc-developers"), http.StatusOK)
		cli.success(t, "poc-reader", "app", "show", "cli-demo", "-o", "json")
	})
	t.Run("authorization commands end to end", func(t *testing.T) {
		testAuthCLI(t, cli)
	})
	t.Run("authorization Bicep deployments end to end", func(t *testing.T) {
		testAuthorizationDeployments(t, cli)
	})
}
