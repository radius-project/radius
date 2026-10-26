// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userauth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/records"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authzcheck"
	"github.com/stretchr/testify/require"
	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

const groupPath = "/planes/radius/local/resourceGroups/team"
const applicationPath = groupPath + "/providers/Applications.Core/applications/demo"
const stagingPath = "/planes/radius/local/resourceGroups/platform/providers/Applications.Core/environments/staging"
const productionPath = "/planes/radius/local/resourceGroups/platform/providers/Applications.Core/environments/production"

func testRuntime(t *testing.T) (*Runtime, http.Handler, *int) {
	t.Helper()
	client := fake.NewClientset()
	client.PrependReactor("create", "subjectaccessreviews", func(action ktesting.Action) (bool, runtime.Object, error) {
		review := action.(ktesting.CreateAction).GetObject().(*authv1.SubjectAccessReview)
		require.Equal(t, "authorizationsettings", review.Spec.ResourceAttributes.Resource)
		require.Equal(t, "update", review.Spec.ResourceAttributes.Verb)
		return true, &authv1.SubjectAccessReview{Status: authv1.SubjectAccessReviewStatus{Allowed: review.Spec.User == "admin"}}, nil
	})
	r, err := New(inmemory.NewClient(), client, Options{Enabled: true, KubernetesIssuer: "kubernetes:test", AcceptIncompleteHistory: true}, "", authz.ModeEnforce)
	require.NoError(t, err)
	writes := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			write(w, http.StatusOK, map[string]any{"id": applicationPath, "properties": map[string]any{"environment": productionPath, "description": "before"}})
			return
		}
		writes++
		w.WriteHeader(http.StatusOK)
	})
	h := r.Middleware(next)
	require.Equal(t, http.StatusCreated, call(t, h, "admin", nil, http.MethodPost, initializePath, nil).Code)
	// Bootstrap is temporary; install a permanent group before enabling.
	require.Equal(t, http.StatusOK, call(t, h, "admin", nil, http.MethodPut,
		"/providers/Radius.Core/roleAssignments/permanent", assignmentBody("radius-administrator", "group", "admins")).Code)
	return r, h, &writes
}

func call(t *testing.T, h http.Handler, user string, groups []string, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req = req.WithContext(authzcheck.WithCaller(req.Context(), authzcheck.Caller{Kind: authzcheck.CallerUser, Name: user, Groups: groups, VerifiedProxy: true}))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	t.Logf("%s %s as %s: %d %s", method, path, user, response.Code, response.Body.String())
	return response
}

func assignmentBody(role, typ, subject string) map[string]any {
	if !strings.HasPrefix(role, "/") {
		role = "/providers/Radius.Core/roleDefinitions/" + role
	}
	return map[string]any{"properties": map[string]any{"roleDefinitionId": role, "principal": map[string]string{"type": typ, "issuer": "kubernetes:test", "subject": subject}}}
}

func setMode(t *testing.T, h http.Handler, mode string) {
	t.Helper()
	response := call(t, h, "admin", []string{"admins"}, http.MethodPut, settingsPath, map[string]any{"properties": map[string]string{"mode": mode}})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
}

func TestModesPolicyProtectionAndRevocation(t *testing.T) {
	t.Parallel()
	r, h, writes := testRuntime(t)
	for _, mode := range []string{"Disabled", "DryRun", "Enforce"} {
		setMode(t, h, mode)
		unauthorizedSettings := call(t, h, "reader", nil, http.MethodPut, settingsPath,
			map[string]any{"properties": map[string]string{"mode": mode}})
		require.Equal(t, http.StatusForbidden, unauthorizedSettings.Code)
		response := call(t, h, "reader", nil, http.MethodPut,
			"/providers/Radius.Core/roleAssignments/attack", assignmentBody("radius-administrator", "user", "reader"))
		require.Equal(t, http.StatusForbidden, response.Code)
		workload := call(t, h, "reader", nil, http.MethodDelete, applicationPath, nil)
		if mode == "Enforce" {
			require.Equal(t, http.StatusForbidden, workload.Code)
		} else {
			require.Equal(t, http.StatusOK, workload.Code)
		}
	}
	require.Equal(t, 2, *writes)
	readerAssignment := groupPath + "/providers/Radius.Core/roleAssignments/readers"
	require.Equal(t, http.StatusOK, call(t, h, "admin", []string{"admins"}, http.MethodPut, readerAssignment, assignmentBody("reader", "group", "readers")).Code)
	require.Equal(t, http.StatusOK, call(t, h, "alice", []string{"readers"}, http.MethodGet, applicationPath, nil).Code)
	require.Equal(t, http.StatusForbidden, call(t, h, "alice", nil, http.MethodGet, applicationPath, nil).Code)
	require.Equal(t, http.StatusNoContent, call(t, h, "admin", []string{"admins"}, http.MethodDelete, readerAssignment, nil).Code)
	require.Equal(t, http.StatusForbidden, call(t, h, "alice", []string{"readers"}, http.MethodGet, applicationPath, nil).Code)
	last := call(t, h, "admin", []string{"admins"}, http.MethodDelete, "/providers/Radius.Core/roleAssignments/permanent", nil)
	require.Equal(t, http.StatusForbidden, last.Code)
	require.Contains(t, last.Body.String(), "LastAdministratorProtected")
	policy, err := r.Store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	value, err := policy.Policy()
	require.NoError(t, err)
	for _, assignment := range value.Assignments {
		require.NotEqual(t, api.AssignmentManagementKindBootstrap, *assignment.Properties.ManagedBy.Kind)
	}
	event, err := r.Store.AccessChange(t.Context(), value.Generation)
	require.NoError(t, err)
	require.Equal(t, "admin", *event.Properties.Principal.Subject)
}

func TestBootstrapAndProvenance(t *testing.T) {
	t.Parallel()
	r, h, _ := testRuntime(t)
	require.Equal(t, http.StatusConflict, call(t, h, "admin", nil, http.MethodPost, initializePath, nil).Code)
	require.Equal(t, http.StatusForbidden, call(t, h, "mallory", nil, http.MethodPut, settingsPath, map[string]any{"properties": map[string]string{"mode": "DryRun"}}).Code)
	body := assignmentBody("reader", "user", "alice")
	body["properties"].(map[string]any)["managedBy"] = map[string]string{"kind": "Bootstrap"}
	require.Equal(t, http.StatusForbidden, call(t, h, "admin", nil, http.MethodPut, groupPath+"/providers/Radius.Core/roleAssignments/spoof", body).Code)
	req := httptest.NewRequest(http.MethodPut, "/providers/Radius.Core/roleAssignments/spoof", bytes.NewBufferString(`{}`))
	req = req.WithContext(authzcheck.WithCaller(context.Background(), authzcheck.Caller{Kind: authzcheck.CallerUser, Name: "admin"}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	r.Options.AcceptIncompleteHistory = false
	response := call(t, h, "admin", nil, http.MethodPut, settingsPath, map[string]any{"properties": map[string]string{"mode": "Enforce"}})
	require.Contains(t, response.Body.String(), "DryRunHistoryIncomplete")
}

func TestPreflightAndEffectivePatch(t *testing.T) {
	t.Parallel()
	_, h, writes := testRuntime(t)
	for _, spec := range []struct{ scope, role string }{{groupPath, "application-developer"}, {stagingPath, "environment-deployer"}} {
		require.Equal(t, http.StatusOK, call(t, h, "admin", nil, http.MethodPut,
			spec.scope+"/providers/Radius.Core/roleAssignments/dev", assignmentBody(spec.role, "user", "developer")).Code)
	}
	setMode(t, h, "Enforce")
	good := map[string]any{"properties": map[string]string{"environment": stagingPath}}
	require.Equal(t, http.StatusOK, call(t, h, "developer", nil, http.MethodPut, applicationPath, good).Code)
	before := *writes
	bad := map[string]any{"properties": map[string]string{"environment": productionPath}}
	require.Equal(t, http.StatusForbidden, call(t, h, "developer", nil, http.MethodPut, applicationPath, bad).Code)
	require.Equal(t, http.StatusForbidden, call(t, h, "developer", nil, http.MethodPatch, applicationPath,
		map[string]any{"properties": map[string]string{"description": "changed"}}).Code)
	deploymentPath := groupPath + "/providers/Microsoft.Resources/deployments/test"
	template := map[string]any{"properties": map[string]any{"mode": "Incremental", "template": map[string]any{
		"resources": []any{map[string]any{"type": "Applications.Core/applications", "name": "demo", "apiVersion": "2023-10-01-preview", "properties": bad["properties"]}},
	}}}
	require.Equal(t, http.StatusForbidden, call(t, h, "developer", nil, http.MethodPut, deploymentPath, template).Code)
	require.Equal(t, before, *writes, "preflight and PATCH denial must happen before any write")
}

func TestApprovalInputBindingAndUnknownTemplate(t *testing.T) {
	t.Parallel()
	r, _, _ := testRuntime(t)
	target := groupPath + "/providers/Microsoft.Resources/deployments/test"
	body := []byte(`{"properties":{"mode":"Incremental","template":{"resources":[{"type":"Applications.Core/applications","apiVersion":"2023-10-01-preview","name":"demo","properties":{"environment":"` + stagingPath + `"}}]}}}`)
	checks, approvals, err := r.writeChecks(target, body)
	require.NoError(t, err)
	require.Len(t, approvals, 2)
	require.Len(t, checks, 2)
	require.Equal(t, records.InputHash([]byte(`{"properties":{"environment":"`+stagingPath+`"}}`)), approvals[1].InputHash)
	for _, bad := range []string{
		strings.Replace(string(body), `"demo"`, `"[parameters('name')]"`, 1),
		strings.Replace(string(body), `"Incremental"`, `"Complete"`, 1),
		strings.Replace(string(body), `"Applications.Core/applications"`, `"Microsoft.Resources/deployments"`, 1),
	} {
		_, _, err := r.writeChecks(target, []byte(bad))
		require.ErrorContains(t, err, "AuthorizationIndeterminate")
	}
}

func TestDelegatedWriteRechecksPolicyAndInput(t *testing.T) {
	t.Parallel()
	r, h, writes := testRuntime(t)
	developerAssignment := groupPath + "/providers/Radius.Core/roleAssignments/developer"
	require.Equal(t, http.StatusOK, call(t, h, "admin", nil, http.MethodPut, developerAssignment,
		assignmentBody("application-developer", "user", "developer")).Code)
	require.Equal(t, http.StatusOK, call(t, h, "admin", nil, http.MethodPut, stagingPath+"/providers/Radius.Core/roleAssignments/use",
		assignmentBody("environment-deployer", "user", "developer")).Code)
	setMode(t, h, "Enforce")
	identity, err := r.identity(authzcheck.WithCaller(t.Context(), authzcheck.Caller{Kind: authzcheck.CallerUser, Name: "developer", VerifiedProxy: true}))
	require.NoError(t, err)
	rawIdentity, err := json.Marshal(identity)
	require.NoError(t, err)
	raw := []byte(`{"properties":{"environment":"` + stagingPath + `"}}`)
	store := records.NewStore(r.DB)
	record := store.NewRecord(records.Record{Subject: "verified-developer", Submitter: authzcheck.ComponentDeploymentEngine,
		Actions: []string{"Applications.Core/applications/write"}, Targets: []string{applicationPath},
		UserApproval: &records.UserApproval{Identity: string(rawIdentity), Resources: []records.ResourceApproval{{
			ResourceID: applicationPath, Action: "Applications.Core/applications/write", InputHash: records.InputHash(raw),
		}}},
	})
	_, err = store.SaveRecord(t.Context(), record, "")
	require.NoError(t, err)
	reloaded, _, err := store.GetRecord(t.Context(), record.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.UserApproval)
	active, err := store.ActiveRecords(t.Context(), authzcheck.ComponentDeploymentEngine)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.NotNil(t, active[0].UserApproval)
	execute := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, applicationPath, bytes.NewReader(body))
		req = req.WithContext(authzcheck.WithCaller(req.Context(), authzcheck.Caller{Kind: authzcheck.CallerComponent, Name: authzcheck.ComponentDeploymentEngine}))
		response := httptest.NewRecorder()
		h.ServeHTTP(response, req)
		return response
	}
	first := execute(raw)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Equal(t, 1, *writes)
	tampered := execute([]byte(`{"properties":{"environment":"` + productionPath + `"}}`))
	require.Equal(t, http.StatusForbidden, tampered.Code)
	require.Contains(t, tampered.Body.String(), "preflight")
	require.Equal(t, http.StatusNoContent, call(t, h, "admin", []string{"admins"}, http.MethodDelete, developerAssignment, nil).Code)
	require.Equal(t, http.StatusForbidden, execute(raw).Code)
	require.Equal(t, 1, *writes, "an old record must not bypass revocation")
}

func TestScopeWriteWithNoReferences(t *testing.T) {
	t.Parallel()
	_, h, _ := testRuntime(t)
	for _, mode := range []string{"DryRun", "Enforce"} {
		setMode(t, h, mode)
		response := call(t, h, "admin", []string{"admins"}, http.MethodPut, groupPath,
			map[string]any{"location": "global", "properties": map[string]any{}})
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}
}

func TestEngineDeploymentUsesRadiusImport(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"properties":{"mode":"Incremental","template":{"resources":[{"type":"Applications.Core/applications","apiVersion":"2023-10-01-preview","name":"demo","location":"global","properties":{"environment":"staging"}}]}}}`)
	converted, err := engineDeploymentBody(raw)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(converted, &body))
	template := body["properties"].(map[string]any)["template"].(map[string]any)
	require.Equal(t, "2.1-experimental", template["languageVersion"])
	resource := template["resources"].(map[string]any)["r0"].(map[string]any)
	require.Equal(t, "radius", resource["import"])
	require.Equal(t, "Applications.Core/applications@2023-10-01-preview", resource["type"])
	envelope := resource["properties"].(map[string]any)
	require.Equal(t, "demo", envelope["name"])
	require.Equal(t, map[string]any{"environment": "staging"}, envelope["properties"])
	_, err = engineDeploymentBody([]byte(`{"properties":{"template":{"imports":{},"resources":[]}}}`))
	require.ErrorContains(t, err, "unsupported literal template field")
}

func TestDryRunDoesNotEnforceUserDelegation(t *testing.T) {
	t.Parallel()
	_, h, writes := testRuntime(t)
	setMode(t, h, "DryRun")
	for _, component := range []string{authzcheck.ComponentController, authzcheck.ComponentDeploymentEngine} {
		req := httptest.NewRequest(http.MethodPut, applicationPath, strings.NewReader(`{"properties":{}}`))
		req = req.WithContext(authzcheck.WithCaller(req.Context(), authzcheck.Caller{Kind: authzcheck.CallerComponent, Name: component}))
		response := httptest.NewRecorder()
		h.ServeHTTP(response, req)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}
	require.Equal(t, 2, *writes)
}

func TestDeploymentProviderScopeAndCanonicalReferences(t *testing.T) {
	target := groupPath + "/providers/Microsoft.Resources/deployments/test"
	prepared, err := deploymentBody(target, []byte(`{"properties":{"mode":"Incremental","template":{"resources":[]}}}`))
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(prepared, &body))
	properties := body["properties"].(map[string]any)
	providers := properties["providerconfig"].(map[string]any)
	require.Equal(t, groupPath, providers["radius"].(map[string]any)["value"].(map[string]any)["scope"])
	_, err = deploymentBody(target, prepared)
	require.NoError(t, err)
	providers["radius"].(map[string]any)["value"].(map[string]any)["scope"] = "/planes/radius/local/resourceGroups/other"
	tampered, err := json.Marshal(body)
	require.NoError(t, err)
	_, err = deploymentBody(target, tampered)
	require.ErrorContains(t, err, "deployment resource group")
	for _, raw := range []string{
		`{"properties":{"Environment":"production"}}`,
		`{"Properties":{"environment":"production"}}`,
		`{"properties":{"connections":{"db":{"Source":"production"}}}}`,
	} {
		var document map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &document))
		require.ErrorContains(t, canonicalFields(document), "canonical JSON field")
	}
}

func TestDiscoveryAndScopeWrite(t *testing.T) {
	t.Parallel()
	r, h, _ := testRuntime(t)
	setMode(t, h, "Enforce")
	_, approvals, err := r.writeChecks(groupPath, []byte(`{"properties":{}}`))
	require.NoError(t, err)
	require.Len(t, approvals, 1)
	for _, path := range []string{"", "/", "/version"} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+strings.TrimPrefix(path, "/"), nil))
		require.Equal(t, http.StatusOK, response.Code)
	}
}
