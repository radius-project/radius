// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	authapi "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type inspectionErrorWriter struct{ header http.Header }

func (w inspectionErrorWriter) Header() http.Header { return w.header }
func (inspectionErrorWriter) WriteHeader(int)       {}
func (inspectionErrorWriter) Write([]byte) (int, error) {
	return 0, errors.New("test response write failure")
}

func TestServiceInspectionErrorLogging(t *testing.T) {
	t.Parallel()
	const pathBase = "/apis/api.ucp.dev/v1alpha3"
	options := frontendRequestOptions(t, pathBase)
	caFile, certificate := frontendProxyCertificate(t, "front-proxy-client")
	options.Config.Server.TLSCertificateDirectory = t.TempDir()
	options.Config.CallerAuthentication = &authorization.RequestHeaderOptions{
		CAFile: caFile, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	require.NoError(t, err)
	core, logs := observer.New(zap.InfoLevel)
	ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))
	server, err := NewService(options).Initialize(ctx)
	require.NoError(t, err)
	const privateScope = "/planes/radius/local/resourceGroups/private-inspection-scope"
	request := httptest.NewRequest(http.MethodPost, pathBase+privateScope+"/providers/Radius.Core/checkAccess?api-version=2025-08-01-preview",
		strings.NewReader(`{"checks":[{"permission":"Radius.Core/applications/read","target":"/planes/radius/local/resourceGroups/private-inspection-scope"}]}`))
	request = request.WithContext(ctx)
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}
	request.Header.Set("X-Remote-User", "bob")
	// No runtime: exercise the inspection error path after trusted authentication.
	server.Handler.ServeHTTP(inspectionErrorWriter{header: http.Header{}}, request)
	events := logs.FilterMessage("Failed to write authorization error response").All()
	require.Len(t, events, 1)
	for _, event := range logs.All() {
		require.NotContains(t, event.ContextMap(), "resourceId")
		encoded, err := json.Marshal(event.ContextMap())
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "private-inspection-scope")
	}
}

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
