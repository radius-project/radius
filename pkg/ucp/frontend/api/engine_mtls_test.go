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
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/authz/token"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/frontend/versions"
	"github.com/radius-project/radius/pkg/validator"
	"github.com/radius-project/radius/swagger"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// These modules stop at the plane boundary: the test verifies authenticated routing,
// not downstream resource execution or execution-record authorization.
type engineTestPlane string

func (p engineTestPlane) PlaneType() string { return string(p) }

func (p engineTestPlane) Initialize(context.Context) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Test-Plane", string(p))
		w.WriteHeader(http.StatusNoContent)
	}), nil
}

func TestInternalServer_DeploymentEngineTokenIdentity(t *testing.T) {
	t.Parallel()

	const namespace = "radius-test"
	const pathBase = "/apis/api.ucp.dev/v1alpha3"
	ca, caKey, caPEM := engineTestCertificate(t, "", nil, nil)
	serverDir, serverCertificate := engineTestMaterial(t, "ucp."+namespace+".svc", ca, caKey, caPEM)
	keys := newEngineTokenKeys(t)
	specLoader, err := validator.LoadSpec(t.Context(), "ucp", swagger.SpecFilesUCP, []string{pathBase}, "")
	require.NoError(t, err)

	for _, provider := range []authz.TLSProvider{authz.TLSProviderRadius, authz.TLSProviderExternal} {
		for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
			t.Run(string(provider)+"/"+string(mode), func(t *testing.T) {
				t.Parallel()
				options := authz.Options{Mode: mode, TLSProvider: provider, CertificateDirectory: serverDir, Namespace: namespace}
				if provider == authz.TLSProviderExternal {
					options.CertificateDirectory = ""
				}
				service := NewService(&ucp.Options{
					Config: &ucp.Config{
						Authorization: options,
						Environment:   hostoptions.EnvironmentOptions{RoleLocation: "local"},
						Server:        hostoptions.ServerOptions{PathBase: pathBase},
					},
					DatabaseProvider: databaseprovider.FromMemory(),
					SecretProvider: secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{
						Provider: secretprovider.TypeInMemorySecret,
					}),
					StatusManager: statusmanager.NewMockStatusManager(gomock.NewController(t)),
					Modules:       []modules.Initializer{engineTestPlane("radius"), engineTestPlane("azure"), engineTestPlane("aws")},
					SpecLoader:    specLoader,
				})
				aggregation, err := service.Initialize(t.Context())
				require.NoError(t, err)
				routes := aggregation.Handler
				aggregation.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if id, ok := authz.IdentityFromContext(r.Context()); ok {
						w.Header().Set("Test-Component", id.String())
					}
					require.Empty(t, r.Header.Get("Authorization"), "receiver token must not reach the business handler")
					if r.TLS != nil {
						require.Empty(t, r.TLS.PeerCertificates, "caller identity must not use a client certificate")
					}
					routes.ServeHTTP(w, r)
				})
				internal := service.internalServer(aggregation)
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				done := make(chan error, 1)
				go func() {
					done <- mtls.Serve(t.Context(), internal, listener, options, mtls.Receiver{Name: "ucp", KeySource: keys})
				}()
				t.Cleanup(func() {
					require.NoError(t, internal.Close())
					require.ErrorIs(t, <-done, http.ErrServerClosed)
				})

				clientOptions := options
				clientOptions.CADirectory = serverDir
				clientOptions.TokenDirectory = engineTokenDirectory(t, "ucp", keys.mint(t, namespace, "bicep-de", "ucp"))
				// A caller needs only public trust and a receiver token, never its own TLS material.
				clientOptions.CertificateDirectory = filepath.Join(t.TempDir(), "no-client-certificate")
				transport, err := mtls.NewClientTransport(clientOptions, "ucp")
				require.NoError(t, err)
				client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
				t.Cleanup(client.CloseIdleConnections)
				scheme := "https"
				if provider == authz.TLSProviderExternal {
					scheme = "http"
				}
				baseURL := scheme + "://" + listener.Addr().String()
				paths := []struct {
					method string
					path   string
					plane  string
					status int
				}{
					{http.MethodGet, "/version", "", http.StatusOK},
					{http.MethodGet, "/planes?api-version=" + versions.DefaultAPIVersion, "", http.StatusOK},
					{http.MethodGet, "/planes/radius/local/resourceGroups/app/providers/System.Test/resources/one?api-version=2025-08-01-preview", "radius", http.StatusNoContent},
					{http.MethodPut, "/planes/radius/local/resourceGroups/app/providers/System.Test/resources/one?api-version=2025-08-01-preview", "radius", http.StatusNoContent},
					{http.MethodDelete, "/planes/radius/local/resourceGroups/app/providers/System.Test/resources/one?api-version=2025-08-01-preview", "radius", http.StatusNoContent},
					{http.MethodPut, "/planes/radius/local/resourceGroups/app/providers/Microsoft.Resources/deployments/nested?api-version=2025-08-01-preview", "radius", http.StatusNoContent},
					{http.MethodGet, "/planes/radius/local/resourceGroups/app/providers/Microsoft.Resources/deployments/nested/operations/operation?api-version=2025-08-01-preview", "radius", http.StatusNoContent},
					{http.MethodPut, "/planes/azure/azurecloud/subscriptions/sub/providers/Microsoft.Test/resources/one?api-version=2025-08-01-preview", "azure", http.StatusNoContent},
					{http.MethodPost, "/planes/aws/aws/accounts/123/regions/us-west-2/providers/AWS.S3/Bucket/:get?api-version=2025-08-01-preview", "aws", http.StatusNoContent},
					{http.MethodDelete, "/planes/aws/aws/accounts/123/regions/us-west-2/providers/AWS.S3/Bucket/one?api-version=2025-08-01-preview", "aws", http.StatusNoContent},
				}
				for _, path := range paths {
					request, err := http.NewRequestWithContext(t.Context(), path.method, baseURL+pathBase+path.path, nil)
					require.NoError(t, err)
					request.Header.Set("Authorization", "Bearer must-be-replaced")
					response, err := client.Do(request)
					require.NoError(t, err, path.path)
					body, err := io.ReadAll(response.Body)
					require.NoError(t, response.Body.Close())
					require.NoError(t, err)
					require.Equal(t, path.status, response.StatusCode, "%s: %s", path.path, body)
					require.Equal(t, "bicep-de."+namespace+".svc", response.Header.Get("Test-Component"), path.path)
					require.Equal(t, path.plane, response.Header.Get("Test-Plane"), path.path)
				}

				for name, bearer := range map[string]string{
					"missing token":     "",
					"wrong namespace":   keys.mint(t, "other", "bicep-de", "ucp"),
					"unknown component": keys.mint(t, namespace, "unknown", "ucp"),
					"wrong audience":    keys.mint(t, namespace, "bicep-de", "bicep-de"),
					"forged token":      "not-a-token",
				} {
					t.Run(name, func(t *testing.T) {
						raw := http.DefaultTransport.(*http.Transport).Clone()
						raw.Proxy = nil
						if provider == authz.TLSProviderRadius {
							raw.TLSClientConfig = &tls.Config{
								MinVersion: tls.VersionTLS12,
								// Pin the URI-only test leaf to exercise requests without an injected token.
								InsecureSkipVerify: true,
								VerifyConnection: func(state tls.ConnectionState) error {
									if len(state.PeerCertificates) == 0 || !bytes.Equal(state.PeerCertificates[0].Raw, serverCertificate.Certificate[0]) {
										return errors.New("unexpected test server certificate")
									}
									return nil
								},
							}
						}
						t.Cleanup(raw.CloseIdleConnections)
						request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+pathBase+"/version", nil)
						require.NoError(t, err)
						if bearer != "" {
							request.Header.Set("Authorization", "Bearer "+bearer)
						}
						response, err := (&http.Client{Transport: raw, Timeout: 5 * time.Second}).Do(request)
						require.NoError(t, err)
						require.NoError(t, response.Body.Close())
						if mode == authz.ModeEnforce {
							require.Equal(t, http.StatusUnauthorized, response.StatusCode)
						} else {
							require.Equal(t, http.StatusOK, response.StatusCode)
						}
						require.Empty(t, response.Header.Get("Test-Component"))
					})
				}
			})
		}
	}
}

type engineTokenKeys struct {
	key *ecdsa.PrivateKey
}

func newEngineTokenKeys(t *testing.T) engineTokenKeys {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return engineTokenKeys{key: key}
}

func (k engineTokenKeys) Fetch(context.Context) (token.KeySet, error) {
	return token.KeySet{Issuer: "https://kubernetes.test", Keys: map[string]crypto.PublicKey{"test": &k.key.PublicKey}}, nil
}

func (k engineTokenKeys) mint(t *testing.T, namespace, account, audience string) string {
	t.Helper()
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": "https://kubernetes.test",
		"aud": []string{audience},
		"sub": "system:serviceaccount:" + namespace + ":" + account,
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(10 * time.Minute).Unix(),
		"kubernetes.io": map[string]any{
			"namespace":      namespace,
			"serviceaccount": map[string]any{"name": account, "uid": "test"},
		},
	})
	tok.Header["kid"] = "test"
	value, err := tok.SignedString(k.key)
	require.NoError(t, err)
	return value
}

func engineTokenDirectory(t *testing.T, receiver, value string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, receiver), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, receiver, mtls.TokenFile), []byte(value), 0o600))
	return dir
}

func engineTestCertificate(t *testing.T, name string, parent *x509.Certificate, signer *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	if parent == nil {
		template.IsCA = true
		template.BasicConstraintsValid = true
		template.KeyUsage |= x509.KeyUsageCertSign
		parent, signer = template, key
	} else {
		component, namespace, ok := strings.Cut(name, ".")
		require.True(t, ok)
		identity, err := url.Parse(mtls.SPIFFEID(mtls.DefaultTrustDomain, strings.TrimSuffix(namespace, ".svc"), component))
		require.NoError(t, err)
		template.URIs = []*url.URL{identity}
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func engineTestMaterial(t *testing.T, name string, ca *x509.Certificate, signer *ecdsa.PrivateKey, caPEM []byte) (string, tls.Certificate) {
	t.Helper()
	_, key, certPEM := engineTestCertificate(t, name, ca, signer)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	dir := t.TempDir()
	for file, data := range map[string][]byte{mtls.CertFile: certPEM, mtls.KeyFile: keyPEM, mtls.CAFile: caPEM} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, file), data, 0o600))
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return dir, cert
}
