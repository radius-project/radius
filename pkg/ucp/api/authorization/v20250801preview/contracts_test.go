// ------------------------------------------------------------
// Copyright 2026 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

package v20250801preview

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/stretchr/testify/require"
)

func TestAssignmentRequestScopes(t *testing.T) {
	t.Parallel()
	client, err := NewScopedRoleAssignmentsClient(nil, nil)
	require.NoError(t, err)
	for _, scope := range []string{
		"planes/radius/local",
		"planes/aws/account",
		"planes/azure/subscription",
		"planes/radius/local/resourceGroups/team-a",
		"planes/radius/local/resourceGroups/platform/providers/Radius.Core/environments/staging",
	} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			resource := RoleAssignmentResource{
				Properties: &RoleAssignmentProperties{
					RoleDefinitionID: to.Ptr("/providers/Radius.Core/roleDefinitions/environment-deployer"),
					Principal: &Principal{
						Type:    to.Ptr(PrincipalTypeGroup),
						Issuer:  to.Ptr("kubernetes:cluster-id"),
						Subject: to.Ptr("team-a-devs"),
					},
				},
			}
			request, err := client.createOrUpdateCreateRequest(t.Context(), scope, "team-a", resource, nil)
			require.NoError(t, err)
			require.Equal(t, http.MethodPut, request.Raw().Method)
			require.Equal(t, "/"+scope+"/providers/Radius.Core/roleAssignments/team-a", request.Raw().URL.Path)
			require.NotContains(t, request.Raw().URL.EscapedPath(), "%2F")
			require.Equal(t, "2025-08-01-preview", request.Raw().URL.Query().Get("api-version"))
			body, err := io.ReadAll(request.Raw().Body)
			require.NoError(t, err)
			require.JSONEq(t, `{"properties":{"roleDefinitionId":"/providers/Radius.Core/roleDefinitions/environment-deployer","principal":{"type":"group","issuer":"kubernetes:cluster-id","subject":"team-a-devs"}}}`, string(body))
			deletion, err := client.deleteCreateRequest(t.Context(), scope, "team-a", &ScopedRoleAssignmentsClientDeleteOptions{TakeOwnership: to.Ptr(true)})
			require.NoError(t, err)
			require.Equal(t, request.Raw().URL.Path, deletion.Raw().URL.Path)
			require.Equal(t, "true", deletion.Raw().URL.Query().Get("takeOwnership"))
		})
	}
}

func TestInstallationAssignmentAndSettingsPaths(t *testing.T) {
	t.Parallel()
	assignment, err := NewInstallationRoleAssignmentsClient(nil, nil)
	require.NoError(t, err)
	request, err := assignment.getCreateRequest(t.Context(), "bootstrap", nil)
	require.NoError(t, err)
	require.Equal(t, "/providers/Radius.Core/roleAssignments/bootstrap", request.Raw().URL.Path)
	settings, err := NewAuthorizationSettingsClient(nil, nil)
	require.NoError(t, err)
	request, err = settings.getCreateRequest(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, "/providers/Radius.Core/authorizationSettings/default", request.Raw().URL.Path)
}

func TestAssignmentResponseRoundTrip(t *testing.T) {
	t.Parallel()
	const response = `{"id":"/planes/radius/local/resourceGroups/team-a/providers/Radius.Core/roleAssignments/team-a","name":"team-a","type":"Radius.Core/roleAssignments","properties":{"roleDefinitionId":"/providers/Radius.Core/roleDefinitions/application-developer","principal":{"type":"group","issuer":"kubernetes:cluster-id","subject":"team-a-devs"},"scope":"/planes/radius/local/resourceGroups/team-a","managedBy":{"kind":"GitOps","source":"https://github.com/contoso/platform-config","revision":"8d2f1e0","path":"access/team-a.bicep"},"createdBy":"user:kubernetes:cluster-id:alice"}}`
	var resource RoleAssignmentResource
	require.NoError(t, json.Unmarshal([]byte(response), &resource))
	require.Equal(t, AssignmentManagementKindGitOps, *resource.Properties.ManagedBy.Kind)
	body, err := json.Marshal(resource)
	require.NoError(t, err)
	require.JSONEq(t, response, string(body))
}

func TestAccessInspectionDoesNotAssertGroups(t *testing.T) {
	t.Parallel()
	client, err := NewInstallationAccessClient(nil, nil)
	require.NoError(t, err)
	request, err := client.checkAccessCreateRequest(t.Context(), AccessCheckRequest{
		Checks: []*AccessCheck{{
			Permission: to.Ptr("Radius.Core/environments/use/action"),
			Target:     to.Ptr("/planes/radius/local/resourceGroups/platform/providers/Radius.Core/environments/staging"),
		}},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "/providers/Radius.Core/checkAccess", request.Raw().URL.Path)
	body, err := io.ReadAll(request.Raw().Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"checks":[{"permission":"Radius.Core/environments/use/action","target":"/planes/radius/local/resourceGroups/platform/providers/Radius.Core/environments/staging"}]}`, string(body))
}
