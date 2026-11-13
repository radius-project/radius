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

package mtls

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServingGenerationRejectsStalePooledHandshake(t *testing.T) {
	t.Parallel()
	for _, h2 := range []bool{false, true} {
		for _, scenario := range []string{"leaf expiry", "intermediate expiry", "anchor retirement", "invalid projection"} {
			t.Run(fmt.Sprintf("%t/%s", h2, scenario), func(t *testing.T) {
				t.Parallel()
				now := time.Now().UTC().Truncate(time.Second)
				root := expiringCA(t, nil, now.Add(4*time.Hour))
				issuer := root
				if scenario == "intermediate expiry" {
					issuer = expiringCA(t, &root, now.Add(10*time.Minute))
				}
				oldLeaf := issuer.issue(t, certOptions{dnsNames: []string{ucpServerName}, notAfter: now.Add(10 * time.Minute)})
				if scenario == "intermediate expiry" {
					oldLeaf = issuer.issue(t, certOptions{dnsNames: []string{ucpServerName}, notAfter: now.Add(time.Hour)})
					oldLeaf.certPEM = append(oldLeaf.certPEM, issuer.pem...)
				}
				dir := newKubeletDir(t, oldLeaf, root.pem)
				source, err := NewSource(dir.root)
				require.NoError(t, err)
				var clock atomic.Int64
				clock.Store(now.UnixNano())
				source.now = func() time.Time { return time.Unix(0, clock.Load()) }
				var served atomic.Int32
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					served.Add(1)
					w.WriteHeader(http.StatusNoContent)
				}))
				source.GuardServingGeneration(server.Config)
				server.TLS, server.EnableHTTP2 = source.ServerConfig(), h2
				if h2 {
					server.TLS.NextProtos = []string{"h2", "http/1.1"}
				}
				server.StartTLS()
				defer server.Close()
				roots := x509.NewCertPool()
				roots.AddCert(root.cert)
				transport := &http.Transport{ForceAttemptHTTP2: h2,
					TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: ucpServerName}}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport}
				request := func(status int, wantReused bool) {
					t.Helper()
					reused := false
					ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{
						GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
					})
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
					require.NoError(t, err)
					response, err := client.Do(req)
					require.NoError(t, err)
					_, err = io.Copy(io.Discard, response.Body)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
					require.Equal(t, status, response.StatusCode)
					require.Equal(t, wantReused, reused)
					require.Equal(t, map[bool]int{false: 1, true: 2}[h2], response.ProtoMajor)
				}
				request(http.StatusNoContent, false)
				renewed := root.issue(t, certOptions{dnsNames: []string{ucpServerName}, notAfter: now.Add(2 * time.Hour)})
				dir.update(t, renewed, root.pem)
				request(http.StatusNoContent, true)
				switch scenario {
				case "leaf expiry", "intermediate expiry":
					clock.Store(now.Add(20 * time.Minute).UnixNano())
				case "anchor retirement":
					replacement := newCA(t)
					dir.update(t, replacement.issue(t, certOptions{dnsNames: []string{ucpServerName}}), replacement.pem)
				case "invalid projection":
					require.NoError(t, os.WriteFile(filepath.Join(dir.root, CAFile), []byte("invalid CA"), 0600))
				}
				request(http.StatusServiceUnavailable, true)
				require.EqualValues(t, 2, served.Load(), "the original handshake must be checked before invoking the handler")
				if scenario == "anchor retirement" || scenario == "invalid projection" {
					dir.update(t, renewed, root.pem)
					request(http.StatusServiceUnavailable, true)
					require.EqualValues(t, 2, served.Load(), "repair must not resurrect an invalidated connection")
				}
			})
		}
	}
}

func TestServingConfig_TrustOverlapAndDelayedAggregator(t *testing.T) {
	t.Parallel()
	for _, protocol := range []string{"http/1.1", "h2"} {
		t.Run(protocol, func(t *testing.T) {
			t.Parallel()
			oldCA, newCA, proxyCA := newCA(t), newCA(t), newCA(t)
			oldLeaf := oldCA.issue(t, certOptions{dnsNames: []string{ucpServerName}})
			newLeaf := newCA.issue(t, certOptions{dnsNames: []string{ucpServerName}})
			proxyLeaf := proxyCA.issue(t, certOptions{dnsNames: []string{"aggregation-proxy.example"}})
			dir := newKubeletDir(t, oldLeaf, oldCA.pem)
			source, err := NewSource(dir.root)
			require.NoError(t, err)
			proxyRoots := x509.NewCertPool()
			proxyRoots.AddCert(proxyCA.cert)
			var authenticated atomic.Int32
			policy := &tls.Config{
				MinVersion: tls.VersionTLS13,
				ClientAuth: tls.RequireAndVerifyClientCert,
				ClientCAs:  proxyRoots,
				VerifyConnection: func(state tls.ConnectionState) error {
					if len(state.VerifiedChains) == 0 {
						return errors.New("aggregation client policy was not preserved")
					}

					authenticated.Add(1)
					return nil
				},
			}
			// A6 can provide a separate aggregation-proxy policy at handshake time.
			base := &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
				return policy, nil
			}}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, r.Proto)
			}))
			server.EnableHTTP2 = protocol == "h2"
			server.TLS = source.ServingConfig(base)
			server.TLS.NextProtos = []string{protocol}
			server.StartTLS()
			defer server.Close()

			roots := x509.NewCertPool()
			roots.AddCert(oldCA.cert)
			transport := &http.Transport{
				ForceAttemptHTTP2: true,
				DisableKeepAlives: true,
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: ucpServerName,
					Certificates: []tls.Certificate{proxyLeaf.tlsCertificate(t)},
				},
			}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport}
			request := func() error {
				response, err := client.Get(server.URL)
				if err != nil {
					return err
				}
				defer response.Body.Close()
				_, err = io.Copy(io.Discard, response.Body)
				require.Equal(t, map[string]string{"h2": "HTTP/2.0", "http/1.1": "HTTP/1.1"}[protocol], response.Proto)
				return err
			}
			reload := func(leaf testCert, bundle []byte) {
				dir.update(t, leaf, bundle)
				_, err := source.Reload()
				require.NoError(t, err)
			}
			require.NoError(t, request())
			overlap := append(bytes.Clone(oldCA.pem), newCA.pem...)
			reload(oldLeaf, overlap)
			require.NoError(t, request(), "publishing trust must preserve the old serving path")

			reload(newLeaf, overlap)
			require.Error(t, request(), "switching before the aggregator loads overlap causes an outage")
			reload(oldLeaf, overlap)
			require.NoError(t, request(), "the old serving path is recoverable before retirement")

			// Model an observed APIService trust update with a fresh aggregator transport.
			transport.CloseIdleConnections()
			updated := transport.Clone()
			defer updated.CloseIdleConnections()
			updated.TLSClientConfig.RootCAs = roots.Clone()
			updated.TLSClientConfig.RootCAs.AddCert(newCA.cert)
			client.Transport = updated
			reload(newLeaf, overlap)
			require.NoError(t, request())
			updated.CloseIdleConnections()
			retired := updated.Clone()
			defer retired.CloseIdleConnections()
			retired.TLSClientConfig.RootCAs = x509.NewCertPool()
			retired.TLSClientConfig.RootCAs.AddCert(newCA.cert)
			client.Transport = retired
			reload(newLeaf, newCA.pem)
			require.NoError(t, request())
			reload(oldLeaf, oldCA.pem)
			require.Error(t, request(), "old serving certificates must fail after explicit retirement")
			require.GreaterOrEqual(t, authenticated.Load(), int32(5))
		})
	}
}
