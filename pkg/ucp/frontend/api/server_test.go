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
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
								w.Header().Set("X-Received-Safe-Trailer", r.Trailer.Get("X-Safe-Trailer"))
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
							proxy := httputil.NewSingleHostReverseProxy(target)
							handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								// Consume trailers before ReverseProxy clones the request.
								body, err := io.ReadAll(r.Body)
								if err != nil {
									t.Errorf("read authenticated body: %v", err)
									w.WriteHeader(http.StatusBadRequest)
									return
								}
								r.Body = io.NopCloser(strings.NewReader(string(body)))
								proxy.ServeHTTP(w, r)
							})
							server, err := NewService(identityTestOptions(t, config, handler)).Initialize(t.Context())
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
												require.Equal(t, "asserted identity", response.Header.Get("X-Received-Safe-Trailer"), "trailer values must reach the backend after body consumption")
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

// writeServingCert writes a CA and a leaf certificate for ucp.radius-system.svc to dir as tls.crt, tls.key, and
// ca.crt, the files of a cert-manager Secret. It returns the leaf certificate.
func writeServingCert(t *testing.T, dir string) *x509.Certificate {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "radius-apiserver-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"ucp.radius-system.svc"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &key.PublicKey, caKey)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	write := func(name, blockType string, der []byte) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600))
	}
	write("tls.crt", "CERTIFICATE", leafDER)
	write("tls.key", "EC PRIVATE KEY", keyDER)
	write("ca.crt", "CERTIFICATE", caDER)

	leaf, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)
	return leaf
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func newTestService(dir string, mode authz.Mode) *Service {
	return &Service{options: &ucp.Options{Config: &ucp.Config{
		Authorization: authz.Options{Mode: mode},
		Server:        hostoptions.ServerOptions{TLSCertificateDirectory: dir},
	}}}
}

func Test_listenAndServe_ServesCertificateFromDirectory(t *testing.T) {
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			dir := t.TempDir()
			leaf := writeServingCert(t, dir)

			addr := "127.0.0.1:" + strconv.Itoa(freePort(t))
			server := &http.Server{Addr: addr, Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
			errs := make(chan error, 1)
			go func() { errs <- newTestService(dir, mode).listenAndServe(t.Context(), server) }()
			t.Cleanup(func() {
				_ = server.Close()
				require.ErrorIs(t, <-errs, http.ErrServerClosed)
			})

			var state tls.ConnectionState
			require.Eventually(t, func() bool {
				conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) //nolint:gosec // The test checks which certificate is served.
				if err != nil {
					return false
				}
				defer conn.Close()
				state = conn.ConnectionState()
				return true
			}, 5*time.Second, 10*time.Millisecond)
			require.Equal(t, leaf.Raw, state.PeerCertificates[0].Raw)
		})
	}
}

func Test_listenAndServe_FailsWithoutCertificateWhenAuthorizationIsOn(t *testing.T) {
	err := newTestService(t.TempDir(), authz.ModeEnforce).listenAndServe(t.Context(), &http.Server{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second})
	require.ErrorContains(t, err, "failed to initialize aggregation serving certificate")
}

// newProxyClientCert returns a CA pool and a client certificate it signs, like the Kubernetes
// API server's front-proxy client certificate.
func newProxyClientCert(t *testing.T) (*x509.CertPool, *x509.Certificate, tls.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(10),
		Subject:               pkix.Name{CommonName: "front-proxy-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(11),
		Subject:      pkix.Name{CommonName: "front-proxy-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}, ca, &key.PublicKey, caKey)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return pool, ca, tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: key}
}

// verifyProxyCertificate applies the request-header verification contract: with
// RequestClientCert, TLS does not verify the presented certificate, so the application verifies
// it against the proxy roots and requires the clientAuth usage.
func verifyProxyCertificate(r *http.Request, roots *x509.CertPool) error {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return errors.New("no client certificate")
	}
	intermediates := x509.NewCertPool()
	for _, cert := range r.TLS.PeerCertificates[1:] {
		intermediates.AddCert(cert)
	}
	_, err := r.TLS.PeerCertificates[0].Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	return err
}

// Test_listenAndServe_PreservesRequestHeaderClientAuth checks that the rotating certificate does
// not replace the request-header proxy's client-certificate settings on the aggregated API server.
func Test_listenAndServe_PreservesRequestHeaderClientAuth(t *testing.T) {
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			dir := t.TempDir()
			leaf := writeServingCert(t, dir)
			roots, proxyCA, proxyCert := newProxyClientCert(t)
			_, _, untrustedCert := newProxyClientCert(t)

			addr := "127.0.0.1:" + strconv.Itoa(freePort(t))
			base := &tls.Config{ClientAuth: tls.RequestClientCert, ClientCAs: roots}
			server := &http.Server{
				Addr:              addr,
				TLSConfig:         base,
				ReadHeaderTimeout: time.Second,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// TLS leaves VerifiedChains empty for RequestClientCert.
					if len(r.TLS.VerifiedChains) != 0 {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					if err := verifyProxyCertificate(r, roots); err != nil {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					w.WriteHeader(http.StatusOK)
				}),
			}
			errs := make(chan error, 1)
			go func() { errs <- newTestService(dir, mode).listenAndServe(t.Context(), server) }()
			t.Cleanup(func() {
				_ = server.Close()
				require.ErrorIs(t, <-errs, http.ErrServerClosed)
			})

			// newClient presents cert, if any, and records the CAs the server advertised.
			newClient := func(cert *tls.Certificate, acceptable *[][]byte) *http.Client {
				return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec // The test checks client-certificate negotiation; VerifyConnection checks the served certificate.
					MinVersion:         tls.VersionTLS12,
					GetClientCertificate: func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
						*acceptable = info.AcceptableCAs
						if cert == nil {
							return &tls.Certificate{}, nil
						}
						return cert, nil
					},
					VerifyConnection: func(cs tls.ConnectionState) error {
						if len(cs.PeerCertificates) == 0 || string(cs.PeerCertificates[0].Raw) != string(leaf.Raw) {
							return errors.New("server did not present the certificate from the directory")
						}
						return nil
					},
				}}}
			}
			status := func(client *http.Client) int {
				var code int
				require.Eventually(t, func() bool {
					resp, err := client.Get("https://" + addr + "/")
					if err != nil {
						return false
					}
					_ = resp.Body.Close()
					code = resp.StatusCode
					return true
				}, 5*time.Second, 10*time.Millisecond)
				return code
			}

			tests := []struct {
				name string
				cert *tls.Certificate
				want int
			}{
				{name: "trusted proxy certificate is verified", cert: &proxyCert, want: http.StatusOK},
				{name: "untrusted certificate is rejected by the application", cert: &untrustedCert, want: http.StatusUnauthorized},
				{name: "caller without a certificate still connects", want: http.StatusUnauthorized},
			}
			for _, tt := range tests {
				var acceptable [][]byte
				require.Equal(t, tt.want, status(newClient(tt.cert, &acceptable)), tt.name)
				// The server requested a client certificate and advertised the proxy CA from ClientCAs.
				require.Equal(t, [][]byte{proxyCA.RawSubject}, acceptable, tt.name)
			}
			require.Equal(t, tls.RequestClientCert, base.ClientAuth)
			require.Nil(t, base.GetConfigForClient)
		})
	}
}
