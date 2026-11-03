// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authapi "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
)

func TestServiceInspectionEveryMode(t *testing.T) {
	t.Parallel()
	for _, mode := range []authapi.UserAuthorizationMode{"", authapi.UserAuthorizationModeDisabled, authapi.UserAuthorizationModeDryRun, authapi.UserAuthorizationModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			const pathBase = "/apis/api.ucp.dev/v1alpha3"
			options := frontendRequestOptions(t, pathBase)
			if mode != "" {
				db, err := options.DatabaseProvider.GetClient(t.Context())
				require.NoError(t, err)
				_, options.UserAuthorization = frontendRequestPolicy(t, db, mode)
			}
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
			for _, subject := range []string{"", "alice", "bob"} {
				for _, operation := range []string{"whoAmI", "checkAccess"} {
					method, body := http.MethodGet, ""
					if operation == "checkAccess" {
						method, body = http.MethodPost, `{"checks":[{"permission":"Radius.Core/applications/read","target":"/"}]}`
					}
					request, err := http.NewRequest(method, listener.URL+pathBase+"/providers/Radius.Core/"+operation+"?api-version=2025-08-01-preview", strings.NewReader(body))
					require.NoError(t, err)
					if subject != "" {
						request.Header.Set("X-Remote-User", subject)
					}
					response, err := client.Do(request)
					require.NoError(t, err)
					data, err := io.ReadAll(response.Body)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
					switch {
					case subject == "":
						require.Equal(t, http.StatusUnauthorized, response.StatusCode, string(data))
					case mode == "":
						require.Equal(t, http.StatusServiceUnavailable, response.StatusCode, string(data))
						require.Contains(t, string(data), "AuthorizationUnavailable")
						require.NotContains(t, string(data), "Disabled")
					default:
						require.Equal(t, http.StatusOK, response.StatusCode, string(data))
						if operation == "whoAmI" {
							var result authapi.WhoAmIResult
							require.NoError(t, json.Unmarshal(data, &result))
							require.Equal(t, subject, *result.Principal.Subject)
							require.Equal(t, subject == "alice", *result.IsRadiusAdministrator)
							require.Equal(t, mode, *result.Mode)
						} else {
							var result authapi.CheckAccessResponse
							require.NoError(t, json.Unmarshal(data, &result))
							require.Equal(t, subject == "alice", *result.Checks[0].Allowed)
							require.Equal(t, mode, *result.Mode)
						}
					}
				}
			}
		})
	}
}
