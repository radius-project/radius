// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userrbac

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const base = "/apis/api.ucp.dev/v1alpha3"
const team = "/planes/radius/local/resourceGroups/rbac-poc-team"
const platform = "/planes/radius/local/resourceGroups/rbac-poc-platform"
const settings = "/providers/Radius.Core/authorizationSettings/default"
const staging = platform + "/providers/Applications.Core/environments/staging"
const production = platform + "/providers/Applications.Core/environments/production"
const app = team + "/providers/Applications.Core/applications/demo"

type apiClient struct {
	t      *testing.T
	config *rest.Config
	client *http.Client
	name   string
}

func (c apiClient) forTest(t *testing.T) apiClient {
	c.t = t
	return c
}

func (c apiClient) request(method, path string, body any, statuses ...int) []byte {
	c.t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(c.t, err)
	version := "2023-10-01-preview"
	if strings.Contains(strings.ToLower(path), "/providers/microsoft.resources/deployments/") {
		version = sdkclients.DeploymentsClientAPIVersion
	}
	if strings.Contains(strings.ToLower(path), "/providers/radius.core/") {
		version = "2025-08-01-preview"
	}
	req, err := http.NewRequestWithContext(c.t.Context(), method, c.config.Host+base+path+"?api-version="+version, bytes.NewReader(raw))
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	require.NoError(c.t, err, "%s %s as %s", method, path, c.name)
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	require.NoError(c.t, err)
	c.t.Logf("%s %s as %s -> HTTP %d: %s", method, path, c.name, response.StatusCode, result)
	require.Contains(c.t, statuses, response.StatusCode, "%s %s as %s: %s", method, path, c.name, result)
	return result
}

func TestKindUserRBAC(t *testing.T) {
	kubeconfig := os.Getenv("RADIUS_USER_RBAC_KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("opt-in: run bash hack/user-rbac/poc.sh test on its dedicated kind cluster")
	}
	contextName := os.Getenv("RADIUS_USER_RBAC_CONTEXT")
	require.True(t, strings.HasPrefix(contextName, "kind-radius-user-rbac-poc"))
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		&clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	require.NoError(t, err)
	config.Timeout = 45 * time.Second
	cli := prepareCLI(t, kubeconfig, contextName)
	kube, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)
	ns, err := kube.CoreV1().Namespaces().Get(t.Context(), "kube-system", metav1.GetOptions{})
	require.NoError(t, err)
	issuer := "kubernetes:" + string(ns.UID)
	client := func(name string, groups ...string) apiClient {
		conf := rest.CopyConfig(config)
		conf.Impersonate = rest.ImpersonationConfig{UserName: name, Groups: append(groups, "system:authenticated")}
		httpClient, err := rest.HTTPClientFor(conf)
		require.NoError(t, err)
		return apiClient{t, conf, httpClient, name}
	}
	admin := client("poc-admin", "poc-admins")
	reader := client("poc-reader", "poc-readers")
	developer := client("poc-developer", "poc-developers")
	outsider := client("poc-outsider")
	principal := func(typ, subject string) map[string]string {
		return map[string]string{"type": typ, "issuer": issuer, "subject": subject}
	}
	assignment := func(role, typ, subject string) map[string]any {
		return map[string]any{"properties": map[string]any{"roleDefinitionId": "/providers/Radius.Core/roleDefinitions/" + role, "principal": principal(typ, subject)}}
	}
	mode := func(value string) {
		admin.request(http.MethodPut, settings, map[string]any{"properties": map[string]string{"mode": value}}, http.StatusOK)
	}
	// A repeat run must use a freshly recreated kit cluster. Initialization is
	// deliberately create-only, including when the proposed policy is identical.
	if os.Getenv("RADIUS_USER_RBAC_RESUME") != "true" {
		admin.request(http.MethodPost, "/providers/Radius.Core/initializeAuthorization", nil, http.StatusCreated)
		admin.request(http.MethodPost, "/providers/Radius.Core/initializeAuthorization", nil, http.StatusConflict)
		admin.request(http.MethodPut, "/providers/Radius.Core/roleAssignments/permanent-admins",
			assignment("radius-administrator", "group", "poc-admins"), http.StatusOK)
		t.Run("Disabled protects policy but passes workloads", func(t *testing.T) {
			outsider := outsider.forTest(t)
			outsider.request(http.MethodPut, "/providers/Radius.Core/roleAssignments/attack",
				assignment("radius-administrator", "user", "poc-outsider"), http.StatusForbidden)
			outsider.request(http.MethodPut, "/planes/radius/local/resourceGroups/rbac-poc-disabled",
				map[string]any{"location": "global", "properties": map[string]any{}}, http.StatusOK, http.StatusCreated)
			outsider.request(http.MethodPut, settings, map[string]any{"properties": map[string]string{"mode": "Enforce"}}, http.StatusForbidden)
		})
		mode("DryRun")
		t.Run("DryRun protects policy but passes workloads", func(t *testing.T) {
			outsider := outsider.forTest(t)
			outsider.request(http.MethodPut, "/providers/Radius.Core/roleDefinitions/attack",
				map[string]any{"properties": map[string]any{"permissions": []string{"Radius.Core/roleAssignments/write"}}}, http.StatusForbidden)
			outsider.request(http.MethodPut, "/planes/radius/local/resourceGroups/rbac-poc-dryrun",
				map[string]any{"location": "global", "properties": map[string]any{}}, http.StatusOK, http.StatusCreated)
		})
	} else {
		t.Log("Resuming owned fixtures through authorized APIs; initialization and Disabled/DryRun scenarios are not repeated.")
		admin.request(http.MethodGet, "/providers/Radius.Core/whoami", nil, http.StatusOK)
	}
	for _, group := range []string{team, platform} {
		admin.request(http.MethodPut, group, map[string]any{"location": "global", "properties": map[string]any{}}, http.StatusOK, http.StatusCreated)
	}
	for _, fixture := range []struct{ id, namespace string }{{staging, "rbac-poc-staging"}, {production, "rbac-poc-production"}} {
		admin.request(http.MethodPut, fixture.id, map[string]any{"location": "global", "properties": map[string]any{
			"compute": map[string]string{"kind": "kubernetes", "resourceId": "self", "namespace": fixture.namespace},
		}}, http.StatusOK, http.StatusCreated, http.StatusAccepted)
		waitProvisioned(t, admin, fixture.id)
	}
	customRole := "/providers/Radius.Core/roleDefinitions/poc-developer"
	admin.request(http.MethodPut, customRole, map[string]any{"properties": map[string]any{
		"displayName": "POC developer", "assignableScopes": []string{team},
		"permissions": []string{"Applications.Core/applications/read", "Applications.Core/applications/write", "Applications.Core/applications/delete",
			"Microsoft.Resources/deployments/write", "Microsoft.Resources/deployments/read"},
	}}, http.StatusOK)
	admin.request(http.MethodPut, team+"/providers/Radius.Core/roleAssignments/developers",
		assignment("poc-developer", "group", "poc-developers"), http.StatusOK)
	admin.request(http.MethodPut, team+"/providers/Radius.Core/roleAssignments/readers",
		assignment("reader", "group", "poc-readers"), http.StatusOK)
	admin.request(http.MethodPut, staging+"/providers/Radius.Core/roleAssignments/staging-deployers",
		assignment("environment-deployer", "group", "poc-developers"), http.StatusOK)
	mode("Enforce")
	admin.request(http.MethodGet, "/providers/Radius.Core/whoami", nil, http.StatusOK)
	outsider.request(http.MethodPut, "/providers/Radius.Core/roleAssignments/attack",
		assignment("radius-administrator", "user", "poc-outsider"), http.StatusForbidden)
	inspection := developer.request(http.MethodPost, "/providers/Radius.Core/checkAccess",
		map[string]any{"checks": []any{
			map[string]string{"permission": "Applications.Core/environments/use/action", "target": staging},
			map[string]string{"permission": "Applications.Core/environments/use/action", "target": production},
		}}, http.StatusOK)
	var access struct {
		Checks []struct {
			Allowed bool `json:"allowed"`
		} `json:"checks"`
	}
	require.NoError(t, json.Unmarshal(inspection, &access))
	require.Len(t, access.Checks, 2)
	require.True(t, access.Checks[0].Allowed)
	require.False(t, access.Checks[1].Allowed)
	body := func(environment string) map[string]any {
		return map[string]any{"location": "global", "properties": map[string]string{"environment": environment}}
	}
	deployment := func(environment, name string) map[string]any {
		return map[string]any{"properties": map[string]any{"mode": "Incremental", "template": map[string]any{
			"$schema":        "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
			"contentVersion": "1.0.0.0", "resources": []any{
				map[string]any{"type": "Applications.Core/applications", "apiVersion": "2023-10-01-preview", "name": name,
					"location": "global", "properties": map[string]string{"environment": environment}},
			},
		}}}
	}
	t.Run("real staging deployment", func(t *testing.T) {
		developer, reader, outsider := developer.forTest(t), reader.forTest(t), outsider.forTest(t)
		developer.request(http.MethodPut, team+"/providers/Microsoft.Resources/deployments/staging",
			deployment(staging, "demo"), http.StatusOK, http.StatusCreated, http.StatusAccepted)
		waitProvisioned(t, developer, app)
		reader.request(http.MethodGet, app, nil, http.StatusOK)
		reader.request(http.MethodPut, app, body(staging), http.StatusForbidden)
		outsider.request(http.MethodGet, app, nil, http.StatusForbidden)
		outsider.request(http.MethodGet, app+"-absent", nil, http.StatusForbidden)
	})
	t.Run("production denied before mutation", func(t *testing.T) {
		admin, developer := admin.forTest(t), developer.forTest(t)
		developer.request(http.MethodPut, team+"/providers/Microsoft.Resources/deployments/production",
			deployment(production, "must-not-exist"), http.StatusForbidden)
		admin.request(http.MethodGet, team+"/providers/Applications.Core/applications/must-not-exist", nil, http.StatusNotFound)
		missing := admin.request(http.MethodGet, team+"/providers/Microsoft.Resources/deployments/production", nil, http.StatusNotFound, http.StatusNoContent)
		if len(missing) != 0 {
			var result struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(missing, &result))
			require.Equal(t, "NotFound", result.Error.Code)
		}
	})
	t.Run("PATCH rechecks omitted environment", func(t *testing.T) {
		admin, developer := admin.forTest(t), developer.forTest(t)
		// This user's write grant remains; remove only the cross-resource use grant.
		admin.request(http.MethodDelete, staging+"/providers/Radius.Core/roleAssignments/staging-deployers", nil, http.StatusNoContent)
		developer.request(http.MethodPatch, app, map[string]any{"tags": map[string]string{"changed": "must-not-apply"}}, http.StatusForbidden)
		result := admin.request(http.MethodGet, app, nil, http.StatusOK)
		require.NotContains(t, string(result), "must-not-apply")
	})
	t.Run("revocation and last admin safety", func(t *testing.T) {
		admin, reader := admin.forTest(t), reader.forTest(t)
		admin.request(http.MethodDelete, team+"/providers/Radius.Core/roleAssignments/readers", nil, http.StatusNoContent)
		reader.request(http.MethodGet, app, nil, http.StatusForbidden)
		result := admin.request(http.MethodDelete, "/providers/Radius.Core/roleAssignments/permanent-admins", nil, http.StatusForbidden)
		require.Contains(t, string(result), "LastAdministratorProtected")
		admin.request(http.MethodGet, "/providers/Radius.Core/accessChanges", nil, http.StatusOK)
	})
	t.Run("forged remote identity is not trusted", func(t *testing.T) {
		// Kubernetes replaces untrusted remote-user headers with the actual
		// authenticated/impersonated identity at the aggregation boundary.
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, outsider.config.Host+base+app+"?api-version=2023-10-01-preview", nil)
		require.NoError(t, err)
		req.Header.Set("X-Remote-User", "poc-admin")
		req.Header.Set("X-Remote-Group", "poc-admins")
		resp, err := outsider.client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
	t.Run("CLI end to end", func(t *testing.T) {
		testCLI(t, cli, kube, admin.forTest(t), assignment)
	})
}

func waitProvisioned(t *testing.T, client apiClient, id string) {
	t.Helper()
	require.Eventually(t, func() bool {
		result := client.request(http.MethodGet, id, nil, http.StatusOK, http.StatusNotFound)
		var value struct {
			Properties struct {
				ProvisioningState string `json:"provisioningState"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(result, &value))
		if strings.EqualFold(value.Properties.ProvisioningState, "Failed") {
			t.Fatalf("resource failed: %s", result)
		}
		return strings.EqualFold(value.Properties.ProvisioningState, "Succeeded")
	}, 3*time.Minute, 2*time.Second, fmt.Sprintf("provision %s", id))
}
