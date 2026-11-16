// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/authz/token"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
)

func TestInternalIdentityPreservesProxyBoundary(t *testing.T) {
	t.Setenv("RADIUS_POD_UID", uuid.NewString())
	service := NewService(&ucp.Options{Config: &ucp.Config{
		Authorization: authz.Options{Mode: authz.ModeDryRun, Namespace: "radius-system"},
	}})
	aggregation := &http.Server{Handler: http.NotFoundHandler()}
	internal := service.internalServer(aggregation)
	for _, forged := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodGet, mtls.InternalIdentityPath+"?challenge="+uuid.NewString(), nil)
		request.TLS = &tls.ConnectionState{HandshakeComplete: true}
		request = request.WithContext(authz.WithIdentity(request.Context(), authz.Identity{Namespace: "radius-system", Component: "bicep-de"}))
		if forged {
			request.Header.Set("X-Remote-User", "admin")
		}
		recorder := httptest.NewRecorder()
		internal.Handler.ServeHTTP(recorder, request)
		if forged {
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
		} else {
			require.Equal(t, http.StatusOK, recorder.Code)
		}
		recorder = httptest.NewRecorder()
		aggregation.Handler.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusNotFound, recorder.Code, "receipt must not be added to aggregation")
	}
}

func TestAggregationComponentTrustSeparation(t *testing.T) {
	t.Parallel()
	proxyCA, proxy := frontendProxyCertificate(t, "front-proxy-client")
	serverDirectory := t.TempDir()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "component-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	spiffe, err := url.Parse(mtls.SPIFFEID(mtls.DefaultTrustDomain, "radius-system", "ucp"))
	require.NoError(t, err)
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), URIs: []*url.URL{spiffe},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &serverKey.PublicKey, caKey)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(serverKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(serverDirectory, mtls.CertFile), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(serverDirectory, mtls.KeyFile), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(serverDirectory, mtls.CAFile), caPEM, 0600))

	signingKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	const issuer = "https://issuer.example.test"
	keys := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var document any
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			document = map[string]string{"issuer": issuer, "jwks_uri": "https://" + r.Host + "/openid/v1/jwks"}
		case "/openid/v1/jwks":
			document = map[string]any{"keys": []map[string]string{{
				"kty": "EC", "crv": "P-256", "kid": "test-key", "alg": "ES256", "use": "sig",
				"x": base64.RawURLEncoding.EncodeToString(signingKey.X.FillBytes(make([]byte, 32))),
				"y": base64.RawURLEncoding.EncodeToString(signingKey.Y.FillBytes(make([]byte, 32))),
			}}}
		default:
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(document); err != nil {
			t.Errorf("write discovery response: %v", err)
		}
	}))
	defer keys.Close()
	mint := func(component, audience string) string {
		now := time.Now()
		tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
			"iss": issuer, "aud": []string{audience},
			"sub": "system:serviceaccount:radius-system:" + component,
			"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(10 * time.Minute).Unix(),
			"kubernetes.io": map[string]any{
				"namespace": "radius-system", "serviceaccount": map[string]string{"name": component, "uid": "test-uid"},
			},
		})
		tok.Header["kid"] = "test-key"
		signed, err := tok.SignedString(signingKey)
		require.NoError(t, err)
		return signed
	}
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
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					t.Errorf("read request: %v", err)
				}
				w.Header().Set("X-Safe-Trailer", r.Trailer.Get("X-Safe-Trailer"))
				if id, ok := authz.IdentityFromContext(r.Context()); ok {
					w.Header().Set("X-Component", id.Component)
					require.Empty(t, r.Header.Get("Authorization"))
					require.Empty(t, r.TLS.PeerCertificates, "components must not present client certificates")
				}
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

			internal := service.internalServer(aggregation)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			done := make(chan error, 1)
			ctx, cancel := context.WithCancel(t.Context())
			go func() {
				done <- mtls.Serve(ctx, internal, listener, config.Authorization, mtls.Receiver{
					Name: "ucp", KeySource: &token.OIDCKeySource{Client: keys.Client(), BaseURL: keys.URL},
				})
			}()
			t.Cleanup(func() {
				cancel()
				require.NoError(t, internal.Close())
				require.ErrorIs(t, <-done, http.ErrServerClosed)
			})

			for _, component := range authz.Components {
				t.Run(component, func(t *testing.T) {
					tokens := t.TempDir()
					require.NoError(t, os.Mkdir(filepath.Join(tokens, "ucp"), 0700))
					require.NoError(t, os.WriteFile(filepath.Join(tokens, "ucp", mtls.TokenFile), []byte(mint(component, "ucp")), 0600))
					options := config.Authorization
					options.TokenDirectory = tokens
					transport, err := mtls.NewClientTransport(options, "ucp")
					require.NoError(t, err)
					defer transport.(interface{ CloseIdleConnections() }).CloseIdleConnections()
					client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
					baseURL := "https://" + listener.Addr().String()
					if mode == authz.ModeOff {
						client = &http.Client{Timeout: 5 * time.Second}
						baseURL = "http://" + listener.Addr().String()
					}
					for _, assertion := range []string{"none", "header", "identity trailer", "safe trailer"} {
						t.Run(assertion, func(t *testing.T) {
							request, err := http.NewRequest(http.MethodPost, baseURL+config.Server.PathBase+"/planes/identity/local", strings.NewReader("body"))
							require.NoError(t, err)
							request.ContentLength = -1
							request.Header.Set("Authorization", "Bearer must-not-be-forwarded")
							if assertion == "header" {
								request.Header.Set("X-Remote-User", "forged-admin")
							}
							if assertion == "identity trailer" {
								request.Trailer = http.Header{"X-Remote-User": []string{"forged-admin"}}
							}
							if assertion == "safe trailer" {
								request.Trailer = http.Header{"X-Safe-Trailer": []string{"retained"}}
							}
							response, err := client.Do(request)
							require.NoError(t, err)
							body, err := io.ReadAll(response.Body)
							require.NoError(t, err)
							require.NoError(t, response.Body.Close())
							if assertion == "header" || assertion == "identity trailer" {
								require.Equal(t, http.StatusUnauthorized, response.StatusCode, string(body))
								require.Contains(t, string(body), "InvalidAuthenticationInfo")
							} else {
								require.Equal(t, http.StatusNoContent, response.StatusCode, string(body))
								if mode != authz.ModeOff {
									require.Equal(t, component, response.Header.Get("X-Component"))
								} else {
									require.Empty(t, response.Header.Get("X-Component"))
								}
								if assertion == "safe trailer" {
									require.Equal(t, "retained", response.Header.Get("X-Safe-Trailer"))
								}
							}
						})
					}
				})
			}

			var certificateRequests atomic.Int64
			directTransport := &http.Transport{TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				// Pin the exact test server leaf; it has a URI SAN, not a DNS name.
				InsecureSkipVerify: true,
				VerifyConnection: func(state tls.ConnectionState) error {
					if len(state.PeerCertificates) == 0 || !bytes.Equal(state.PeerCertificates[0].Raw, der) {
						return errors.New("unexpected test server certificate")
					}
					return nil
				},
				GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
					certificateRequests.Add(1)
					return &proxy, nil
				},
			}}
			defer directTransport.CloseIdleConnections()
			direct := &http.Client{Transport: directTransport, Timeout: 5 * time.Second}
			internalURL := "https://" + listener.Addr().String()
			if mode == authz.ModeOff {
				internalURL = "http://" + listener.Addr().String()
			}
			for _, tt := range []struct {
				name  string
				token string
			}{
				{name: "proxy certificate without token"},
				{name: "malformed token", token: "not-a-token"},
				{name: "wrong audience", token: mint("ucp", "applications-rp")},
			} {
				t.Run(tt.name, func(t *testing.T) {
					request, err := http.NewRequest(http.MethodGet, internalURL+config.Server.PathBase+"/planes/identity/local", nil)
					require.NoError(t, err)
					if tt.token != "" {
						request.Header.Set("Authorization", "Bearer "+tt.token)
					}
					response, err := direct.Do(request)
					require.NoError(t, err)
					body, err := io.ReadAll(response.Body)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
					require.Empty(t, response.Header.Get("X-Component"))
					if mode == authz.ModeEnforce {
						require.Equal(t, http.StatusUnauthorized, response.StatusCode, string(body))
						require.Contains(t, string(body), "InvalidAuthenticationInfo")
					} else {
						require.Equal(t, http.StatusNoContent, response.StatusCode, string(body))
					}
				})
			}
			require.Zero(t, certificateRequests.Load(), "internal TLS must not request a client certificate")

			// A valid component token cannot replace the aggregation proxy certificate.
			request, err := http.NewRequest(http.MethodGet, public.URL+config.Server.PathBase+"/planes/identity/local", nil)
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer "+mint("ucp", "ucp"))
			request.Header.Set("X-Remote-User", "forged-admin")
			response, err := public.Client().Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusUnauthorized, response.StatusCode)

			transport := public.Client().Transport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			transport.TLSClientConfig.Certificates = []tls.Certificate{proxy}
			request, err = http.NewRequest(http.MethodGet, public.URL+config.Server.PathBase+"/planes/identity/local", nil)
			require.NoError(t, err)
			request.Header.Set("X-Remote-User", "alice")
			response, err = (&http.Client{Transport: transport, Timeout: 5 * time.Second}).Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusOK, response.StatusCode, "dedicated proxy trust still establishes identity")
		})
	}
}
