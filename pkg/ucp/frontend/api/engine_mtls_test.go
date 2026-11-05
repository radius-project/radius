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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
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

func TestInternalServer_DeploymentEngineIdentity(t *testing.T) {
	t.Parallel()

	const namespace = "radius-test"
	const pathBase = "/apis/api.ucp.dev/v1alpha3"
	ca, caKey, caPEM := engineTestCertificate(t, "", nil, nil)
	serverDir, _ := engineTestMaterial(t, "ucp."+namespace+".svc", ca, caKey, caPEM)
	_, engineCert := engineTestMaterial(t, "bicep-de."+namespace+".svc", ca, caKey, caPEM)
	_, otherNamespace := engineTestMaterial(t, "bicep-de.other.svc", ca, caKey, caPEM)
	_, unknownComponent := engineTestMaterial(t, "unknown."+namespace+".svc", ca, caKey, caPEM)
	otherCA, otherKey, otherPEM := engineTestCertificate(t, "", nil, nil)
	_, untrusted := engineTestMaterial(t, "bicep-de."+namespace+".svc", otherCA, otherKey, otherPEM)
	specLoader, err := validator.LoadSpec(t.Context(), "ucp", swagger.SpecFilesUCP, []string{pathBase}, "")
	require.NoError(t, err)

	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			options := authz.Options{Mode: mode, CertificateDirectory: serverDir, Namespace: namespace}
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
				if id, ok := mtls.IdentityFromContext(r.Context()); ok {
					w.Header().Set("Test-Component", id.String())
				}
				routes.ServeHTTP(w, r)
			})
			internal := service.internalServer(aggregation)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() { done <- mtls.Serve(t.Context(), internal, listener, options, nil) }()
			t.Cleanup(func() {
				require.NoError(t, internal.Close())
				require.ErrorIs(t, <-done, http.ErrServerClosed)
			})

			roots := x509.NewCertPool()
			require.True(t, roots.AppendCertsFromPEM(caPEM))
			newClient := func(cert *tls.Certificate) *http.Client {
				config := &tls.Config{
					MinVersion: tls.VersionTLS12,
					ServerName: "ucp." + namespace + ".svc",
					RootCAs:    roots,
				}
				if cert != nil {
					config.Certificates = []tls.Certificate{*cert}
				}
				client := &http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 5 * time.Second}
				t.Cleanup(client.CloseIdleConnections)
				return client
			}
			client := newClient(&engineCert)
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
				request, err := http.NewRequestWithContext(t.Context(), path.method, "https://"+listener.Addr().String()+pathBase+path.path, nil)
				require.NoError(t, err)
				response, err := client.Do(request)
				require.NoError(t, err, path.path)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, response.Body.Close())
				require.NoError(t, err)
				require.Equal(t, path.status, response.StatusCode, "%s: %s", path.path, body)
				require.Equal(t, "bicep-de."+namespace+".svc", response.Header.Get("Test-Component"), path.path)
				require.Equal(t, path.plane, response.Header.Get("Test-Plane"), path.path)
			}

			for name, cert := range map[string]*tls.Certificate{
				"missing certificate": nil,
				"wrong namespace":     &otherNamespace,
				"unknown component":   &unknownComponent,
				"wrong CA":            &untrusted,
			} {
				t.Run(name, func(t *testing.T) {
					response, err := newClient(cert).Get("https://" + listener.Addr().String() + pathBase + "/version")
					if mode == authz.ModeEnforce {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
					require.Equal(t, http.StatusOK, response.StatusCode)
					require.Empty(t, response.Header.Get("Test-Component"))
				})
			}
		})
	}
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
		template.DNSNames = []string{name}
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
