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
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

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
