// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
)

func TestAggregationComponentTrustSeparation(t *testing.T) {
	t.Parallel()
	proxyCA, proxy := frontendProxyCertificate(t, "front-proxy-client")
	componentCAKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "component-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &componentCAKey.PublicKey, componentCAKey)
	require.NoError(t, err)
	componentCA, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	issue := func(t *testing.T, component string) (string, tls.Certificate) {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(2), DNSNames: []string{component + ".radius-system.svc"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, componentCA, &key.PublicKey, componentCAKey)
		require.NoError(t, err)
		keyDER, err := x509.MarshalECPrivateKey(key)
		require.NoError(t, err)
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, mtls.CertFile), certPEM, 0600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, mtls.KeyFile), keyPEM, 0600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, mtls.CAFile), caPEM, 0600))
		certificate, err := tls.X509KeyPair(certPEM, keyPEM)
		require.NoError(t, err)
		return dir, certificate
	}
	serverDirectory, _ := issue(t, "ucp")
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			config := &ucp.Config{
				Authorization: authz.Options{Mode: mode, CertificateDirectory: serverDirectory, Namespace: "radius-system"},
				CallerAuthentication: &authorization.RequestHeaderOptions{
					CAFile: proxyCA, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
				},
				Server: hostoptions.ServerOptions{TLSCertificateDirectory: t.TempDir(), PathBase: "/apis/api.ucp.dev/v1alpha3"},
			}
			service := NewService(identityTestOptions(t, config, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, identified := authorization.IdentityFromContext(r.Context()); identified {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
			})))
			aggregation, err := service.Initialize(t.Context())
			require.NoError(t, err)
			public := httptest.NewUnstartedServer(aggregation.Handler)
			public.TLS = aggregation.TLSConfig
			public.StartTLS()
			defer public.Close()

			// This is the real internal listener/mode adapter, not an injected
			// VerifiedChains value or the aggregation listener's TLS policy.
			internal := service.internalServer(aggregation)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			done := make(chan error, 1)
			ctx, cancel := context.WithCancel(t.Context())
			go func() { done <- mtls.Serve(ctx, internal, listener, config.Authorization, nil) }()
			t.Cleanup(func() {
				cancel()
				require.NoError(t, internal.Close())
				require.ErrorIs(t, <-done, http.ErrServerClosed)
			})

			for _, component := range []string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"} {
				t.Run(component, func(t *testing.T) {
					_, certificate := issue(t, component)
					for _, endpoint := range []string{"aggregation", "internal"} {
						t.Run(endpoint, func(t *testing.T) {
							var transport *http.Transport
							var baseURL string
							if endpoint == "aggregation" {
								baseURL = public.URL
								transport = public.Client().Transport.(*http.Transport).Clone()
							} else {
								baseURL = "http://" + listener.Addr().String()
								transport = &http.Transport{}
								if mode != authz.ModeOff {
									baseURL = "https://" + listener.Addr().String()
									pool := x509.NewCertPool()
									require.True(t, pool.AppendCertsFromPEM(caPEM))
									transport.TLSClientConfig = &tls.Config{
										MinVersion: tls.VersionTLS12, RootCAs: pool, ServerName: "ucp.radius-system.svc",
									}
								}
							}
							defer transport.CloseIdleConnections()
							if transport.TLSClientConfig != nil {
								// Force presentation even when the proxy CA hint does
								// not match: a withheld cert is not a wrong-CA test.
								transport.TLSClientConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
									return &certificate, nil
								}
							}
							client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
							for _, asserted := range []bool{false, true} {
								request, err := http.NewRequest(http.MethodGet, baseURL+config.Server.PathBase+"/planes/identity/local", nil)
								require.NoError(t, err)
								if asserted {
									request.Header.Set("X-Remote-User", "forged-admin")
								}
								response, err := client.Do(request)
								require.NoError(t, err)
								body, err := io.ReadAll(response.Body)
								require.NoError(t, err)
								require.NoError(t, response.Body.Close())
								if asserted {
									require.Equal(t, http.StatusUnauthorized, response.StatusCode, string(body))
									require.Contains(t, string(body), "InvalidAuthenticationInfo")
								} else {
									require.Equal(t, http.StatusNoContent, response.StatusCode, string(body))
								}
							}
						})
					}
				})
			}

			transport := public.Client().Transport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			transport.TLSClientConfig.Certificates = []tls.Certificate{proxy}
			request, err := http.NewRequest(http.MethodGet, public.URL+config.Server.PathBase+"/planes/identity/local", nil)
			require.NoError(t, err)
			request.Header.Set("X-Remote-User", "alice")
			response, err := (&http.Client{Transport: transport, Timeout: 5 * time.Second}).Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusOK, response.StatusCode, "dedicated proxy trust still establishes identity")
		})
	}
}
