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
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/stretchr/testify/require"
)

func TestAggregationServingProofPreservesProxyPolicy(t *testing.T) {
	clearAggregationProofEnv(t)
	ca, key, caPEM := engineTestCertificate(t, "", nil, nil)
	proxyCA, proxyKey, proxyPEM := engineTestCertificate(t, "", nil, nil)
	dir := t.TempDir()
	aggregationServingMaterial(t, dir, "ucp.radius-system.svc", ca, key, caPEM)
	digest, _, err := mtls.TrustBundleDigest(caPEM, time.Now())
	require.NoError(t, err)
	t.Setenv("UCP_AGGREGATION_STARTUP_PROOF_ENABLED", "true")
	t.Setenv("UCP_AGGREGATION_EXPECTED_TRUST_SHA256", digest)
	t.Setenv("UCP_AGGREGATION_POD_UID", uuid.NewString())
	service := NewService(&ucp.Options{Config: &ucp.Config{
		Server:        hostoptions.ServerOptions{TLSCertificateDirectory: dir},
		Authorization: authz.Options{Namespace: "radius-system"}}})
	_, err = service.PrepareAggregationProof(true)
	require.NoError(t, err)
	proxyRoots := x509.NewCertPool()
	proxyRoots.AddCert(proxyCA)
	aggregation := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: proxyRoots}, nil
		}}}
	internal := service.internalServer(aggregation)
	require.Nil(t, internal.TLSConfig)
	_, err = service.configureAggregationServing(aggregation)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(aggregation.Handler)
	server.TLS = aggregation.TLSConfig
	server.StartTLS()
	defer server.Close()
	servingRoots := x509.NewCertPool()
	servingRoots.AddCert(ca)
	_, proxyClient := engineTestMaterial(t, "aggregation-proxy", proxyCA, proxyKey, proxyPEM)
	_, componentClient := engineTestMaterial(t, "controller.radius-system.svc", ca, key, caPEM)
	for _, tt := range []struct {
		name         string
		certificates []tls.Certificate
		allowed      bool
	}{
		{"proxy client", []tls.Certificate{proxyClient}, true},
		{"component client", []tls.Certificate{componentClient}, false},
		{"no client", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12,
				RootCAs: servingRoots, ServerName: "ucp.radius-system.svc", Certificates: tt.certificates}}
			defer transport.CloseIdleConnections()
			response, err := (&http.Client{Transport: transport}).Get(server.URL)
			if !tt.allowed {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusNoContent, response.StatusCode)
			require.NotEmpty(t, response.Header.Get(mtls.AggregationPodUIDHeader))
		})
	}
}
