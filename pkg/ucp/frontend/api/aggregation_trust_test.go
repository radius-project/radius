/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/frontend/versions"
	"github.com/radius-project/radius/pkg/validator"
	"github.com/radius-project/radius/swagger"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAggregationEndpoint_HeaderlessCompatibility(t *testing.T) {
	t.Parallel()

	const pathBase = "/apis/api.ucp.dev/v1alpha3"
	specLoader, err := validator.LoadSpec(t.Context(), "ucp", swagger.SpecFilesUCP, []string{pathBase}, "")
	require.NoError(t, err)

	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			service := NewService(&ucp.Options{
				Config: &ucp.Config{
					Authorization: authz.Options{Mode: mode},
					Environment:   hostoptions.EnvironmentOptions{RoleLocation: "local"},
					Server: hostoptions.ServerOptions{
						PathBase: pathBase,
					},
				},
				DatabaseProvider: databaseprovider.FromMemory(),
				SecretProvider: secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{
					Provider: secretprovider.TypeInMemorySecret,
				}),
				StatusManager: statusmanager.NewMockStatusManager(gomock.NewController(t)),
				Modules:       []modules.Initializer{},
				SpecLoader:    specLoader,
			})
			aggregation, err := service.Initialize(t.Context())
			require.NoError(t, err)

			// Until A7 migrates the deployment engine, the aggregation listener
			// must still serve headerless requests without a client certificate.
			server := httptest.NewUnstartedServer(aggregation.Handler)
			server.TLS = aggregation.TLSConfig
			server.StartTLS()
			t.Cleanup(server.Close)
			client := server.Client()
			t.Cleanup(client.CloseIdleConnections)

			for _, path := range []string{"/version", "/planes?api-version=" + versions.DefaultAPIVersion} {
				response, err := client.Get(server.URL + service.options.Config.Server.PathBase + path)
				require.NoError(t, err)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, response.Body.Close())
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, response.StatusCode, "%s: %s", path, body)
			}
		})
	}
}

func TestInternalServer_RejectsForwardedIdentity(t *testing.T) {
	t.Parallel()

	caFile, proxy := frontendProxyCertificate(t, "front-proxy-client")
	leaf, err := x509.ParseCertificate(proxy.Certificate[0])
	require.NoError(t, err)
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			config := &ucp.Config{
				Authorization: authz.Options{Mode: mode},
				CallerAuthentication: &authorization.RequestHeaderOptions{
					CAFile: caFile, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
				},
				Server: hostoptions.ServerOptions{TLSCertificateDirectory: t.TempDir(), PathBase: "/apis/api.ucp.dev/v1alpha3"},
			}
			service := NewService(identityTestOptions(t, config, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, identified := authorization.IdentityFromContext(r.Context())
				if identified {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
			})))
			aggregation, err := service.Initialize(t.Context())
			require.NoError(t, err)
			internal := service.internalServer(aggregation)
			for _, tt := range []struct {
				name    string
				headers bool
				trailer bool
				status  int
			}{
				{name: "headerless", status: http.StatusNoContent},
				{name: "forwarded user", headers: true, status: http.StatusUnauthorized},
				{name: "announced identity trailer", trailer: true, status: http.StatusUnauthorized},
			} {
				t.Run(tt.name, func(t *testing.T) {
					request := httptest.NewRequest(http.MethodGet, config.Server.PathBase+"/planes/identity/local", nil)
					// Even a proxy-trusted peer must not establish user identity on
					// the component endpoint, which has a different trust boundary.
					request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}
					if tt.headers {
						request.Header.Set("X-Remote-User", "alice")
					}
					if tt.trailer {
						request.Trailer = http.Header{"X-Remote-User": nil}
					}
					response := httptest.NewRecorder()
					internal.Handler.ServeHTTP(response, request)
					require.Equal(t, tt.status, response.Code, response.Body.String())
					if tt.status == http.StatusUnauthorized {
						require.Contains(t, response.Body.String(), "InvalidAuthenticationInfo")
					}
					if tt.headers {
						response = httptest.NewRecorder()
						aggregation.Handler.ServeHTTP(response, request)
						require.Equal(t, http.StatusOK, response.Code, "aggregation must still accept its verified proxy")
					}
				})
			}
		})
	}
}

func TestInternalServer_DoesNotInheritAggregationTLSPolicy(t *testing.T) {
	t.Parallel()

	service := NewService(&ucp.Options{Config: &ucp.Config{
		Server: hostoptions.ServerOptions{Host: "127.0.0.1", InternalPort: 9444},
	}})
	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "shared routes")
	called := false
	proxyTLS := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: tls.RequestClientCert,
		ClientCAs:  x509.NewCertPool(),
	}
	aggregation := &http.Server{
		Addr:      "127.0.0.1:9443",
		TLSConfig: proxyTLS,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			require.Equal(t, "shared routes", r.Context().Value(contextKey{}))
			w.WriteHeader(http.StatusNoContent)
		}),
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	internal := service.internalServer(aggregation)
	require.Equal(t, "127.0.0.1:9444", internal.Addr)
	// Component trust is supplied by mtls.ListenAndServe, not by the proxy CA.
	require.Nil(t, internal.TLSConfig)
	require.Same(t, proxyTLS, aggregation.TLSConfig)
	require.Equal(t, tls.RequestClientCert, aggregation.TLSConfig.ClientAuth)
	require.Equal(t, "127.0.0.1:9443", aggregation.Addr)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/version", nil)
	internal.Handler.ServeHTTP(response, request.WithContext(internal.BaseContext(nil)))
	require.True(t, called)
	require.Equal(t, http.StatusNoContent, response.Code)
}
