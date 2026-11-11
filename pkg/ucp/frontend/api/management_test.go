// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	authapi "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
)

type managementTransport struct {
	client  *http.Client
	subject string
}

func (transport managementTransport) Do(request *http.Request) (*http.Response, error) {
	request.Header.Set("X-Remote-User", transport.subject)
	// No client header establishes deployment or GitOps provenance.
	request.Header.Set("X-Radius-Managed-By", "GitOps")
	return transport.client.Do(request)
}

func TestServiceManagementGeneratedClientsEveryMode(t *testing.T) {
	t.Parallel()
	for _, mode := range []authapi.UserAuthorizationMode{authapi.UserAuthorizationModeDisabled, authapi.UserAuthorizationModeDryRun, authapi.UserAuthorizationModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			const pathBase = "/apis/api.ucp.dev/v1alpha3"
			options := frontendRequestOptions(t, pathBase)
			db, err := options.DatabaseProvider.GetClient(t.Context())
			require.NoError(t, err)
			store, runtime := frontendRequestPolicy(t, db, mode)
			options.UserAuthorization = runtime
			caFile, certificate := frontendProxyCertificate(t, "front-proxy-client")
			options.Config.Server.TLSCertificateDirectory = t.TempDir()
			options.Config.CallerAuthentication = &authorization.RequestHeaderOptions{
				CAFile: caFile, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
			}
			server, err := NewService(options).Initialize(t.Context())
			require.NoError(t, err)
			listener := httptest.NewUnstartedServer(server.Handler)
			listener.TLS = server.TLSConfig
			listener.StartTLS()
			defer listener.Close()
			transport := listener.Client().Transport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			transport.TLSClientConfig.Certificates = []tls.Certificate{certificate}
			client := &http.Client{Transport: transport}
			clientOptions := func(subject string) *arm.ClientOptions {
				return &arm.ClientOptions{DisableRPRegistration: true, ClientOptions: policy.ClientOptions{
					Transport: managementTransport{client: client, subject: subject},
					Cloud: cloud.Configuration{Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
						cloud.ResourceManager: {Endpoint: listener.URL + pathBase, Audience: "https://radius.test"},
					}},
					Retry: policy.RetryOptions{MaxRetries: -1},
				}}
			}
			credential := &aztoken.AnonymousCredential{}
			roles, err := authapi.NewInstallationRoleDefinitionsClient(credential, clientOptions("alice"))
			require.NoError(t, err)
			assignments, err := authapi.NewInstallationRoleAssignmentsClient(credential, clientOptions("alice"))
			require.NoError(t, err)
			role, err := roles.CreateOrUpdate(t.Context(), "operator", authapi.RoleDefinitionResource{
				Properties: &authapi.RoleDefinitionProperties{Permissions: to.SliceOfPtrs("Radius.Core/applications/read")},
			}, nil)
			require.NoError(t, err)
			require.Equal(t, "Radius.Core/roleDefinitions", *role.Type)
			require.Equal(t, authapi.RoleTypeCustom, *role.Properties.RoleType)
			assignment, err := assignments.CreateOrUpdate(t.Context(), "operator", authapi.RoleAssignmentResource{
				Properties: &authapi.RoleAssignmentProperties{
					RoleDefinitionID: role.ID, Principal: &authapi.Principal{
						Type: to.Ptr(authapi.PrincipalTypeGroup), Issuer: to.Ptr("kubernetes:test-cluster"), Subject: to.Ptr("operators"),
					},
				},
			}, nil)
			require.NoError(t, err)
			require.Equal(t, authapi.AssignmentManagementKindImperative, *assignment.Properties.ManagedBy.Kind)
			require.Equal(t, "user:kubernetes:test-cluster:alice", *assignment.Properties.CreatedBy)
			get, err := assignments.Get(t.Context(), "operator", nil)
			require.NoError(t, err)
			require.Equal(t, assignment.RoleAssignmentResource, get.RoleAssignmentResource)
			pager := roles.NewListPager(nil)
			page, err := pager.NextPage(t.Context())
			require.NoError(t, err)
			require.Len(t, page.Value, 10)
			require.False(t, pager.More())
			assignmentPage, err := assignments.NewListPager(&authapi.InstallationRoleAssignmentsClientListOptions{
				Filter: to.Ptr("atScopeAndBelow()"),
			}).NextPage(t.Context())
			require.NoError(t, err)
			require.Len(t, assignmentPage.Value, 2)

			// A management endpoint in an Azure/AWS plane stays in UCP even
			// though ordinary provider requests on those planes are proxied.
			scoped, err := authapi.NewScopedRoleDefinitionsClient(credential, clientOptions("alice"))
			require.NoError(t, err)
			scopedAssignments, err := authapi.NewScopedRoleAssignmentsClient(credential, clientOptions("alice"))
			require.NoError(t, err)
			for _, scope := range []string{"planes/azure/local", "planes/aws/local", "planes/radius/local/resourceGroups/team-a"} {
				scopedRole, err := scoped.CreateOrUpdate(t.Context(), scope, "operator", authapi.RoleDefinitionResource{
					Properties: &authapi.RoleDefinitionProperties{Permissions: to.SliceOfPtrs("Radius.Core/applications/read")},
				}, nil)
				require.NoError(t, err)
				require.Equal(t, "/"+strings.ToLower(scope)+"/providers/radius.core/roledefinitions/operator", *scopedRole.ID)
				_, err = scopedAssignments.CreateOrUpdate(t.Context(), scope, "operator", authapi.RoleAssignmentResource{
					Properties: &authapi.RoleAssignmentProperties{
						RoleDefinitionID: scopedRole.ID, Principal: &authapi.Principal{
							Type: to.Ptr(authapi.PrincipalTypeGroup), Issuer: to.Ptr("kubernetes:test-cluster"), Subject: to.Ptr("operators"),
						},
					},
				}, nil)
				require.NoError(t, err)
				_, err = scopedAssignments.Delete(t.Context(), scope, "operator", nil)
				require.NoError(t, err)
				_, err = scoped.Delete(t.Context(), scope, "operator", nil)
				require.NoError(t, err)
			}

			for _, subject := range []string{"bob", ""} {
				denied, err := authapi.NewInstallationRoleAssignmentsClient(credential, clientOptions(subject))
				require.NoError(t, err)
				_, err = denied.Delete(t.Context(), "operator", nil)
				var responseError *azcore.ResponseError
				require.ErrorAs(t, err, &responseError)
				expected := http.StatusForbidden
				if subject == "" {
					expected = http.StatusUnauthorized
				}
				require.Equal(t, expected, responseError.StatusCode)
			}
			_, err = assignments.Delete(t.Context(), "admin", nil)
			var lastAdmin *azcore.ResponseError
			require.ErrorAs(t, err, &lastAdmin)
			require.Equal(t, "LastAdministratorProtected", lastAdmin.ErrorCode)
			_, err = assignments.Delete(t.Context(), "operator", nil)
			require.NoError(t, err)
			_, err = roles.Delete(t.Context(), "operator", nil)
			require.NoError(t, err)
			snapshot, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			require.Equal(t, int64(16), snapshot.Generation())
			history, err := authapi.NewAccessChangesClient(credential, clientOptions("alice"))
			require.NoError(t, err)
			historyPage, err := history.NewListPager(nil).NextPage(t.Context())
			require.NoError(t, err)
			require.Len(t, historyPage.Value, 16)
			require.Nil(t, historyPage.NextLink)
			event, err := history.Get(t.Context(), "2", nil)
			require.NoError(t, err)
			require.Equal(t, "Radius.Core/accessChanges", *event.Type)
			require.Equal(t, "2", *event.Name)
			require.Equal(t, "alice", *event.Properties.Principal.Subject)
			require.Equal(t, assignment.RoleAssignmentResource.Properties, event.Properties.PolicyChange.After.RoleAssignment)
			require.Equal(t, assignment.ID, event.Properties.PolicyChange.Target)
			filtered, err := history.NewListPager(&authapi.AccessChangesClientListOptions{
				Scope:     to.Ptr("/planes/radius/local/resourceGroups/team-a"),
				Since:     event.Properties.Time,
				Principal: to.Ptr(`{"type":"user","issuer":"kubernetes:test-cluster","subject":"alice"}`),
			}).NextPage(t.Context())
			require.NoError(t, err)
			require.Len(t, filtered.Value, 4)
			deniedHistory, err := authapi.NewAccessChangesClient(credential, clientOptions("bob"))
			require.NoError(t, err)
			_, err = deniedHistory.Get(t.Context(), "2", nil)
			var historyError *azcore.ResponseError
			require.ErrorAs(t, err, &historyError)
			require.Equal(t, http.StatusForbidden, historyError.StatusCode)
		})
	}
}
