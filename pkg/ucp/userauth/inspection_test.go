// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package userauth

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

func TestIdentityAndEffectiveAccess(t *testing.T) {
	t.Parallel()
	_, handler, _ := testRuntime(t)
	setMode(t, handler, "Enforce")
	for _, spec := range []struct {
		name   string
		user   string
		groups []string
		admin  bool
	}{
		{"group administrator", "bob", []string{"admins"}, true},
		{"outsider", "alice", []string{"readers"}, false},
	} {
		t.Run(spec.name, func(t *testing.T) {
			response := call(t, handler, spec.user, spec.groups, http.MethodGet, "/providers/Radius.Core/whoami", nil)
			require.Equal(t, http.StatusOK, response.Code)
			var value api.WhoAmIResult
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &value))
			require.Equal(t, "kubernetes:test", *value.Issuer)
			require.Equal(t, spec.user, *value.Principal.Subject)
			require.Equal(t, spec.admin, *value.IsRadiusAdministrator)
			require.Len(t, value.Groups, len(spec.groups))
		})
	}
	assignment := groupPath + "/providers/Radius.Core/roleAssignments/inspect-reader"
	require.Equal(t, http.StatusOK, call(t, handler, "admin", []string{"admins"}, http.MethodPut,
		assignment, assignmentBody("reader", "group", "readers")).Code)
	principal := `{"type":"group","issuer":"kubernetes:test","subject":"readers"}`
	path := applicationPath + "/providers/Radius.Core/effectiveAccess?principal=" + url.QueryEscape(principal)
	response := call(t, handler, "admin", []string{"admins"}, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var access api.EffectiveAccessResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &access))
	require.True(t, strings.EqualFold(applicationPath, *access.Scope))
	require.Len(t, access.Assignments, 1)
	require.True(t, *access.Assignments[0].Inherited)
	require.True(t, strings.EqualFold(assignment, *access.Assignments[0].Assignment.ID))
	require.NotEmpty(t, access.Permissions)
	require.Equal(t, http.StatusForbidden, call(t, handler, "alice", []string{"readers"}, http.MethodGet, path, nil).Code)
	malformed := applicationPath + "/providers/Radius.Core/effectiveAccess?principal=invalid"
	require.NotEqual(t, http.StatusOK, call(t, handler, "admin", []string{"admins"}, http.MethodGet, malformed, nil).Code)
	require.Equal(t, http.StatusNoContent, call(t, handler, "admin", []string{"admins"}, http.MethodDelete, assignment, nil).Code)
	response = call(t, handler, "admin", []string{"admins"}, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, response.Code)
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &access))
	require.Empty(t, access.Assignments)
	require.Empty(t, access.Permissions)
}

func TestExplanationRedactsUnreadableGrants(t *testing.T) {
	t.Parallel()
	_, handler, _ := testRuntime(t)
	setMode(t, handler, "Enforce")
	assignment := groupPath + "/providers/Radius.Core/roleAssignments/inspect-reader"
	require.Equal(t, http.StatusOK, call(t, handler, "admin", []string{"admins"}, http.MethodPut,
		assignment, assignmentBody("reader", "group", "readers")).Code)
	checks := map[string]any{"checks": []map[string]string{{"permission": "Applications.Core/applications/read", "target": applicationPath}}}
	response := call(t, handler, "alice", []string{"readers"}, http.MethodPost, groupPath+"/providers/Radius.Core/explainAccess", checks)
	require.Equal(t, http.StatusOK, response.Code)
	var explanation api.ExplainAccessResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &explanation))
	require.True(t, *explanation.Checks[0].Allowed)
	require.Empty(t, explanation.Checks[0].Grants)
	checks["principal"] = map[string]string{"type": "group", "issuer": "kubernetes:test", "subject": "readers"}
	response = call(t, handler, "admin", []string{"admins"}, http.MethodPost, groupPath+"/providers/Radius.Core/explainAccess", checks)
	require.Equal(t, http.StatusOK, response.Code)
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &explanation))
	require.Len(t, explanation.Checks[0].Grants, 1)
	require.True(t, strings.EqualFold(assignment, *explanation.Checks[0].Grants[0].AssignmentID))
	require.Equal(t, http.StatusForbidden, call(t, handler, "alice", []string{"readers"}, http.MethodPost,
		groupPath+"/providers/Radius.Core/explainAccess", checks).Code)
}

func TestInspectionCannotEscapeItsAuthorizedScope(t *testing.T) {
	t.Parallel()
	_, handler, _ := testRuntime(t)
	body := map[string]any{"properties": map[string]any{
		"permissions":      []string{"Radius.Core/checkAccess/action", "Radius.Core/explainAccess/action"},
		"assignableScopes": []string{groupPath},
	}}
	role := groupPath + "/providers/Radius.Core/roleDefinitions/inspector"
	require.Equal(t, http.StatusOK, call(t, handler, "admin", []string{"admins"}, http.MethodPut, role, body).Code)
	require.Equal(t, http.StatusOK, call(t, handler, "admin", []string{"admins"}, http.MethodPut,
		groupPath+"/providers/Radius.Core/roleAssignments/inspector", assignmentBody(role, "user", "inspector")).Code)
	setMode(t, handler, "Enforce")
	for _, action := range []string{"checkAccess", "explainAccess"} {
		response := call(t, handler, "inspector", nil, http.MethodPost, groupPath+"/providers/Radius.Core/"+action, map[string]any{
			"principal": map[string]string{"type": "user", "issuer": "kubernetes:test", "subject": "other"},
			"checks":    []map[string]string{{"permission": "Applications.Core/environments/read", "target": productionPath}},
		})
		require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	}

}

func TestAssignmentMetadataUpdatesPreserveIdentityAndProvenance(t *testing.T) {
	t.Parallel()
	_, handler, _ := testRuntime(t)
	setMode(t, handler, "Enforce")
	path := groupPath + "/providers/Radius.Core/roleAssignments/labels"
	body := assignmentBody("reader", "user", "alice")
	first := call(t, handler, "admin", []string{"admins"}, http.MethodPut, path, body)
	require.Equal(t, http.StatusOK, first.Code)
	var original api.RoleAssignmentResource
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &original))
	properties := body["properties"].(map[string]any)
	properties["description"] = "Temporary project access"
	principal := properties["principal"].(map[string]string)
	principal["displayName"] = "Alice's project account"
	response := call(t, handler, "admin", []string{"admins"}, http.MethodPut, path, body)
	require.Equal(t, http.StatusOK, response.Code)
	var updated api.RoleAssignmentResource
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &updated))
	require.Equal(t, "Temporary project access", *updated.Properties.Description)
	require.Equal(t, "Alice's project account", *updated.Properties.Principal.DisplayName)
	require.Equal(t, original.Properties.ManagedBy, updated.Properties.ManagedBy)
	require.Equal(t, original.Properties.CreatedBy, updated.Properties.CreatedBy)
	require.Equal(t, original.Properties.Scope, updated.Properties.Scope)
	require.Equal(t, original.Properties.RoleDefinitionID, updated.Properties.RoleDefinitionID)
	require.Equal(t, original.Properties.Principal.Subject, updated.Properties.Principal.Subject)
	before := call(t, handler, "admin", []string{"admins"}, http.MethodGet, settingsPath, nil)
	require.Equal(t, http.StatusOK, call(t, handler, "admin", []string{"admins"}, http.MethodPut, path, body).Code)
	after := call(t, handler, "admin", []string{"admins"}, http.MethodGet, settingsPath, nil)
	require.JSONEq(t, before.Body.String(), after.Body.String(), "repeating identical metadata must not commit another generation")
	principal["subject"] = "mallory"
	response = call(t, handler, "admin", []string{"admins"}, http.MethodPut, path, body)
	require.Equal(t, http.StatusForbidden, response.Code)
	require.Contains(t, response.Body.String(), "RoleAssignmentImmutable")
}
