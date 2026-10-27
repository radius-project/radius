// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userauth

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/radius-project/radius/pkg/authz/records"
	"github.com/stretchr/testify/require"
)

func policyTemplate(permission string) map[string]any {
	return map[string]any{"properties": map[string]any{
		"mode": "Incremental",
		"template": map[string]any{"resources": []any{
			map[string]any{"type": "Radius.Core/roleDefinitions", "apiVersion": "2025-08-01-preview", "name": "template-reader",
				"properties": map[string]any{"permissions": []string{permission}}},
			map[string]any{"type": "Radius.Core/roleAssignments", "apiVersion": "2025-08-01-preview", "name": "template-reader",
				"scope": groupPath + "/providers/Radius.Core/applications/demo",
				"properties": map[string]any{
					"roleDefinitionId": groupPath + "/providers/Radius.Core/roleDefinitions/template-reader",
					"principal":        map[string]string{"type": "user", "issuer": "kubernetes:test", "subject": "outsider"},
				}},
			map[string]any{"type": "Radius.Core/applications", "apiVersion": "2025-08-01-preview", "name": "demo",
				"properties": map[string]any{"environment": stagingPath}},
		}},
	}}
}

func TestDeploymentPolicyPreflightBeforeAnyResources(t *testing.T) {
	t.Parallel()
	for _, valid := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid plan changes nothing", true: "policy and workloads"}[valid], func(t *testing.T) {
			t.Parallel()
			r, h, writes := testRuntime(t)
			setMode(t, h, "Enforce")
			before, err := r.Store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			engineRequests := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				engineRequests++
				var body map[string]any
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				template := body["properties"].(map[string]any)["template"].(map[string]any)
				require.Len(t, template["resources"].(map[string]any), 1, "components must not administer policy")
				approval := records.UserApprovalFromContext(req.Context())
				require.NotNil(t, approval)
				require.Len(t, approval.Resources, 2, "only deployment and workload inputs are delegated")
				latest, err := r.Store.LoadPolicy(req.Context(), -1)
				require.NoError(t, err)
				require.Equal(t, before.Generation()+2, latest.Generation())
				w.WriteHeader(http.StatusOK)
			})
			permission := "Radius.Core/applications/read"
			if !valid {
				permission = "Radius.Core/unregistered/write"
			}
			response := call(t, r.Middleware(next), "admin", []string{"admins"}, http.MethodPut,
				groupPath+"/providers/Microsoft.Resources/deployments/policy", policyTemplate(permission))
			after, err := r.Store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			if valid {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Equal(t, 1, engineRequests)
				visible := call(t, h, "outsider", nil, http.MethodGet, groupPath+"/providers/Radius.Core/applications/demo", nil)
				require.Equal(t, http.StatusOK, visible.Code)
			} else {
				require.NotEqual(t, http.StatusOK, response.Code)
				require.Equal(t, 0, engineRequests)
				require.Equal(t, before.Generation(), after.Generation())
				require.Zero(t, *writes)
			}
		})
	}
}

func TestDeploymentCannotBootstrapItsOwnAuthority(t *testing.T) {
	t.Parallel()
	r, h, writes := testRuntime(t)
	setMode(t, h, "Enforce")
	require.Equal(t, http.StatusOK, call(t, h, "admin", []string{"admins"}, http.MethodPut,
		groupPath+"/providers/Radius.Core/roleAssignments/developer", assignmentBody("application-developer", "user", "developer")).Code)
	before, err := r.Store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	response := call(t, h, "developer", nil, http.MethodPut, groupPath+"/providers/Microsoft.Resources/deployments/escalate",
		policyTemplate("Radius.Core/applications/read"))
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	after, err := r.Store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	require.Equal(t, before.Generation(), after.Generation())
	require.Zero(t, *writes)
}

func TestResolvedWorkloadDependenciesAndLiteralEscaping(t *testing.T) {
	t.Parallel()
	body, err := deploymentBody(groupPath+"/providers/Microsoft.Resources/deployments/test", []byte(`{
		"properties":{"mode":"Incremental","template":{"resources":{
			"application":{"import":"radius","type":"Radius.Core/applications@2025-08-01-preview","properties":{"name":"app","properties":{"environment":"`+stagingPath+`","description":"[[literal]"}}},
			"child":{"import":"radius","type":"Radius.Core/applications@2025-08-01-preview","properties":{"name":"other","properties":{"environment":"`+stagingPath+`"}},"dependsOn":["application"]}
		}}}
	}`))
	require.NoError(t, err)
	body, err = engineDeploymentBody(body)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	items := parsed["properties"].(map[string]any)["template"].(map[string]any)["resources"].(map[string]any)
	require.Equal(t, []any{"r0"}, items["r1"].(map[string]any)["dependsOn"])
	require.Equal(t, "[[literal]", items["r0"].(map[string]any)["properties"].(map[string]any)["properties"].(map[string]any)["description"])
}
