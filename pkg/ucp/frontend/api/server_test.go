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
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type identityTestModule struct {
	handler http.Handler
}

func (m identityTestModule) PlaneType() string { return "identity" }

func (m identityTestModule) Initialize(context.Context) (http.Handler, error) {
	return m.handler, nil
}

func identityTestOptions(t *testing.T, config *ucp.Config, handler http.Handler) *ucp.Options {
	t.Helper()
	config.Environment.RoleLocation = "local"
	return &ucp.Options{
		Config: config, DatabaseProvider: databaseprovider.FromMemory(),
		SecretProvider: secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{Provider: secretprovider.TypeInMemorySecret}),
		Modules:        []modules.Initializer{identityTestModule{handler: handler}},
		StatusManager:  statusmanager.NewMockStatusManager(gomock.NewController(t)),
	}
}

func TestServiceRequestHeaderTLS(t *testing.T) {
	t.Parallel()
	caFile, clientCertificate := frontendProxyCertificate(t, "front-proxy-client")
	_, untrustedCertificate := frontendProxyCertificate(t, "front-proxy-client")
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			config := &ucp.Config{
				Authorization: authz.Options{Mode: mode},
				CallerAuthentication: &authorization.RequestHeaderOptions{
					CAFile: caFile, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
				},
				Server: hostoptions.ServerOptions{
					TLSCertificateDirectory: t.TempDir(), PathBase: "/apis/api.ucp.dev/v1alpha3",
				},
			}
			options := identityTestOptions(t, config, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				identity, ok := authorization.IdentityFromContext(r.Context())
				for header := range r.Header {
					require.NotContains(t, header, "X-Remote-")
				}
				if ok {
					require.Equal(t, "alice", *identity.Principal.Subject)
					require.Equal(t, "kubernetes:test-cluster", *identity.Principal.Issuer)
					require.Len(t, identity.Groups, 1)
					require.Equal(t, "team-a", *identity.Groups[0].Subject)
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			server, err := NewService(options).Initialize(t.Context())
			require.NoError(t, err)
			require.Equal(t, tls.RequestClientCert, server.TLSConfig.ClientAuth)
			listener := httptest.NewUnstartedServer(server.Handler)
			listener.TLS = server.TLSConfig
			listener.StartTLS()
			defer listener.Close()
			for _, tt := range []struct {
				name    string
				cert    *tls.Certificate
				headers bool
				status  int
			}{
				{name: "verified proxy", cert: &clientCertificate, headers: true, status: http.StatusOK},
				{name: "spoofed headers with no cert", headers: true, status: http.StatusUnauthorized},
				{name: "untrusted cert", cert: &untrustedCertificate, headers: true, status: http.StatusUnauthorized},
				{name: "headerless compatibility", status: http.StatusNoContent},
				{name: "certificate alone is not caller identity", cert: &clientCertificate, status: http.StatusNoContent},
			} {
				t.Run(tt.name, func(t *testing.T) {
					transport := listener.Client().Transport.(*http.Transport).Clone()
					defer transport.CloseIdleConnections()
					transport.TLSClientConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
						if tt.cert != nil {
							return tt.cert, nil
						}
						return &tls.Certificate{}, nil
					}
					client := &http.Client{Transport: transport}
					request, err := http.NewRequest(http.MethodGet, listener.URL+config.Server.PathBase+"/planes/identity/local", nil)
					require.NoError(t, err)
					if tt.headers {
						request.Header.Set("X-Remote-User", "alice")
						request.Header.Set("X-Remote-Group", "team-a")
						request.Header.Set("X-Remote-Extra-Admin", "true")
					}
					response, err := client.Do(request)
					require.NoError(t, err)
					defer response.Body.Close()
					body, err := io.ReadAll(response.Body)
					require.NoError(t, err)
					require.Equal(t, tt.status, response.StatusCode, string(body))
					if tt.status == http.StatusUnauthorized {
						require.Contains(t, string(body), "InvalidAuthenticationInfo")
					}
				})
			}
		})
	}
}

func TestServiceRequestHeaderConfiguration(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		config ucp.Config
		error  string
	}{
		{name: "configured HTTP fails startup", config: ucp.Config{CallerAuthentication: &authorization.RequestHeaderOptions{}}, error: "requires a TLS server certificate directory"},
		{name: "invalid config fails startup", config: ucp.Config{
			CallerAuthentication: &authorization.RequestHeaderOptions{},
			Server:               hostoptions.ServerOptions{TLSCertificateDirectory: t.TempDir()},
		}, error: "requires a kubernetes:<trust-domain> issuer"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService(identityTestOptions(t, &tt.config, nil)).Initialize(t.Context())
			require.ErrorContains(t, err, tt.error)
		})
	}
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run("unconfigured "+string(mode), func(t *testing.T) {
			server, err := NewService(identityTestOptions(t, &ucp.Config{Authorization: authz.Options{Mode: mode}}, nil)).Initialize(t.Context())
			require.NoError(t, err)
			require.Nil(t, server.TLSConfig)
			for _, headers := range []bool{false, true} {
				request := httptest.NewRequest(http.MethodGet, "/version", nil)
				if headers {
					request.Header.Set("X-Remote-User", "admin")
				}
				response := httptest.NewRecorder()
				server.Handler.ServeHTTP(response, request)
				if headers {
					require.Equal(t, http.StatusUnauthorized, response.Code)
				} else {
					require.Equal(t, http.StatusOK, response.Code)
				}
			}
		})
	}
}

func TestServiceRequestHeaderTrailers(t *testing.T) {
	t.Parallel()
	caFile, certificate := frontendProxyCertificate(t, "front-proxy-client")
	_, untrustedCertificate := frontendProxyCertificate(t, "front-proxy-client")
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			for _, protocol := range []int{1, 2} {
				t.Run(fmt.Sprintf("HTTP%d", protocol), func(t *testing.T) {
					for _, peer := range []struct {
						name       string
						configured bool
						cert       *tls.Certificate
					}{
						{name: "unconfigured"},
						{name: "missing certificate", configured: true},
						{name: "unverified certificate", configured: true, cert: &untrustedCertificate},
						{name: "verified certificate", configured: true, cert: &certificate},
					} {
						t.Run(peer.name, func(t *testing.T) {
							var forwarded atomic.Int64
							backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								forwarded.Add(1)
								if _, err := io.Copy(io.Discard, r.Body); err != nil {
									t.Errorf("read proxied body: %v", err)
								}
								w.WriteHeader(http.StatusNoContent)
							}))
							defer backend.Close()
							target, err := url.Parse(backend.URL)
							require.NoError(t, err)
							config := &ucp.Config{
								Authorization: authz.Options{Mode: mode},
								Server: hostoptions.ServerOptions{
									TLSCertificateDirectory: t.TempDir(), PathBase: "/apis/api.ucp.dev/v1alpha3",
								},
							}
							if peer.configured {
								config.CallerAuthentication = &authorization.RequestHeaderOptions{
									CAFile: caFile, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
								}
							}
							server, err := NewService(identityTestOptions(t, config, httputil.NewSingleHostReverseProxy(target))).Initialize(t.Context())
							require.NoError(t, err)
							type receivedRequest struct {
								protocol int
								trailers http.Header
								chunked  bool
								peers    int
							}
							received := make(chan receivedRequest, 1)
							listener := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								// Record announcements before authentication or body consumption.
								received <- receivedRequest{
									protocol: r.ProtoMajor, trailers: r.Trailer.Clone(),
									chunked: len(r.TransferEncoding) == 1 && r.TransferEncoding[0] == "chunked",
									peers:   len(r.TLS.PeerCertificates),
								}
								server.Handler.ServeHTTP(w, r)
							}))
							listener.TLS = server.TLSConfig
							listener.EnableHTTP2 = protocol == 2
							listener.StartTLS()
							defer listener.Close()
							transport := listener.Client().Transport.(*http.Transport).Clone()
							defer transport.CloseIdleConnections()
							transport.ForceAttemptHTTP2 = protocol == 2
							// Isolate early rejection from connection shutdown on prior streams.
							transport.DisableKeepAlives = true
							transport.TLSClientConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
								if peer.cert != nil {
									return peer.cert, nil
								}
								return &tls.Certificate{}, nil
							}
							client := &http.Client{Transport: transport}
							for _, trailer := range []string{"x-remote-user", "X-REMOTE-GROUP", "x-ReMoTe-ExTrA-Admin", "X-Remote-Uid", "X-Safe-Trailer"} {
								t.Run(trailer, func(t *testing.T) {
									for _, headers := range []bool{false, true} {
										t.Run(fmt.Sprintf("identity headers=%t", headers), func(t *testing.T) {
											before := forwarded.Load()
											request, err := http.NewRequest(http.MethodPost, listener.URL+config.Server.PathBase+"/planes/identity/local",
												io.NopCloser(strings.NewReader("request body")))
											require.NoError(t, err)
											request.ContentLength = -1
											request.Trailer = http.Header{}
											request.Trailer.Set(trailer, "asserted identity")
											if headers {
												request.Header.Set("X-Remote-User", "alice")
											}
											response, err := client.Do(request)
											require.NoError(t, err)
											defer response.Body.Close()
											body, err := io.ReadAll(response.Body)
											require.NoError(t, err)
											observed := <-received
											require.Equal(t, protocol, observed.protocol)
											require.Equal(t, protocol == 1, observed.chunked)
											require.Contains(t, observed.trailers, http.CanonicalHeaderKey(trailer))
											require.Empty(t, observed.trailers.Values(trailer), "only announcement is available before reading the body")
											if peer.cert != nil {
												require.Positive(t, observed.peers)
											}
											forbidden := trailer != "X-Safe-Trailer" || (headers && peer.name != "verified certificate")
											if forbidden {
												require.Equal(t, http.StatusUnauthorized, response.StatusCode, string(body))
												require.Contains(t, string(body), "InvalidAuthenticationInfo")
												require.Equal(t, before, forwarded.Load(), "identity trailers must not reach the reverse proxy")
											} else {
												require.Equal(t, http.StatusNoContent, response.StatusCode, string(body))
												require.Equal(t, before+1, forwarded.Load(), "unrelated trailers remain supported")
											}
										})
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

func frontendProxyCertificate(t *testing.T, name string) (string, tls.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "request-header-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "request-header-ca.crt")
	require.NoError(t, os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600))
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	require.NoError(t, err)
	return file, tls.Certificate{Certificate: [][]byte{der, caDER}, PrivateKey: key}
}
