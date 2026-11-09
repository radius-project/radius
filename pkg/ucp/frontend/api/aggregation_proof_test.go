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
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/stretchr/testify/require"
)

func aggregationServingMaterial(t *testing.T, dir, name string, ca *x509.Certificate, signer *ecdsa.PrivateKey, caPEM []byte) string {
	t.Helper()
	leaf, key, _ := engineTestCertificate(t, name, ca, signer)
	leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, signer)
	require.NoError(t, err)
	private, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	for file, data := range map[string][]byte{
		mtls.CertFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		mtls.KeyFile:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), mtls.CAFile: caPEM,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, file), data, 0600))
	}
	hash := sha256.Sum256(der)
	return hex.EncodeToString(hash[:])
}

func clearAggregationProofEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"UCP_AGGREGATION_STARTUP_PROOF_ENABLED", "UCP_AGGREGATION_EXPECTED_TRUST_SHA256",
		"UCP_AGGREGATION_POD_UID", "UCP_MTLS_STARTUP_PROOF_ENABLED", "UCP_MTLS_EXPECTED_TRUST_SHA256", "UCP_MTLS_POD_UID"} {
		t.Setenv(name, "")
	}
}

func TestAggregationServingProofRuntime(t *testing.T) {
	clearAggregationProofEnv(t)
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			for _, h2 := range []bool{false, true} {
				t.Run(map[bool]string{false: "http1", true: "http2"}[h2], func(t *testing.T) {
					ca, key, caPEM := engineTestCertificate(t, "", nil, nil)
					dir, podUID := t.TempDir(), uuid.NewString()
					digest, _, err := mtls.TrustBundleDigest(caPEM, time.Now())
					require.NoError(t, err)
					leaf := aggregationServingMaterial(t, dir, "ucp.radius-system.svc", ca, key, caPEM)
					t.Setenv("UCP_AGGREGATION_STARTUP_PROOF_ENABLED", "true")
					t.Setenv("UCP_AGGREGATION_EXPECTED_TRUST_SHA256", digest)
					t.Setenv("UCP_AGGREGATION_POD_UID", podUID)
					service := NewService(&ucp.Options{Config: &ucp.Config{
						Server: hostoptions.ServerOptions{TLSCertificateDirectory: dir},
						// No component CertificateDirectory: serving proof works
						// independently of component PKI and authorization mode.
						Authorization: authz.Options{Mode: mode, Namespace: "radius-system"}}})
					proof, err := service.PrepareAggregationProof(true)
					require.NoError(t, err)
					mux := http.NewServeMux()
					mux.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
					aggregation := &http.Server{Handler: mux}
					internal := service.internalServer(aggregation)
					source, err := service.configureAggregationServing(aggregation)
					require.NoError(t, err)
					require.Same(t, service.aggregationSource, source)
					server := httptest.NewUnstartedServer(aggregation.Handler)
					server.TLS, server.EnableHTTP2 = aggregation.TLSConfig, h2
					if h2 {
						server.TLS.NextProtos = []string{"h2", "http/1.1"}
					}
					server.StartTLS()
					defer server.Close()
					roots := x509.NewCertPool()
					roots.AddCert(ca)
					transport := &http.Transport{ForceAttemptHTTP2: h2,
						TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "ucp.radius-system.svc"}}
					defer transport.CloseIdleConnections()
					client := &http.Client{Transport: transport}
					observe := func(expectedLeaf string) {
						t.Helper()
						status := httptest.NewRecorder()
						proof.ServeHTTP(status, httptest.NewRequest(http.MethodGet, mtls.AggregationProofPath, nil))
						require.Equal(t, http.StatusOK, status.Code)
						var evidence mtls.AggregationEvidence
						require.NoError(t, json.Unmarshal(status.Body.Bytes(), &evidence))
						require.NoError(t, evidence.Validate(podUID, digest, expectedLeaf, time.Now()))
						transport.CloseIdleConnections()
						response, err := client.Get(server.URL + "/test")
						require.NoError(t, err)
						_, err = io.Copy(io.Discard, response.Body)
						require.NoError(t, err)
						require.NoError(t, response.Body.Close())
						require.Equal(t, map[bool]int{false: 1, true: 2}[h2], response.ProtoMajor)
						hash := sha256.Sum256(response.TLS.PeerCertificates[0].Raw)
						require.Equal(t, expectedLeaf, hex.EncodeToString(hash[:]))
						require.Equal(t, podUID, response.Header.Get(mtls.AggregationPodUIDHeader))
					}
					observe(leaf)
					renewed := aggregationServingMaterial(t, dir, "ucp.radius-system.svc", ca, key, caPEM)
					require.NotEqual(t, leaf, renewed)
					observe(renewed)
					internalResponse := httptest.NewRecorder()
					request := httptest.NewRequest(http.MethodGet, "/test", nil)
					request.Header.Set(mtls.AggregationPodUIDHeader, "caller-controlled")
					internal.Handler.ServeHTTP(internalResponse, request)
					require.Empty(t, internalResponse.Header().Get(mtls.AggregationPodUIDHeader))
					require.Nil(t, internal.TLSConfig)
					for _, handler := range []http.Handler{internal.Handler, aggregation.Handler} {
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, mtls.AggregationProofPath, nil))
						require.Equal(t, http.StatusNotFound, response.Code, "proof is management-only")
					}
					aggregationServingMaterial(t, dir, "controller.radius-system.svc", ca, key, caPEM)
					_, err = proof.Snapshot()
					require.Error(t, err)
					transport.CloseIdleConnections()
					_, err = client.Get(server.URL + "/test")
					require.Error(t, err, "invalid proof renewal invalidates actual serving Source")
				})
			}
		})
	}
}

func TestAggregationServingProofConfiguration(t *testing.T) {
	for _, scenario := range []string{"default off", "disabled settings", "invalid boolean", "missing metrics",
		"missing directory", "unreadable directory", "different pod"} {
		t.Run(scenario, func(t *testing.T) {
			clearAggregationProofEnv(t)
			service := NewService(&ucp.Options{Config: &ucp.Config{}})
			management := true
			switch scenario {
			case "disabled settings":
				t.Setenv("UCP_AGGREGATION_POD_UID", uuid.NewString())
			case "invalid boolean":
				t.Setenv("UCP_AGGREGATION_STARTUP_PROOF_ENABLED", "yes")
			case "missing metrics":
				t.Setenv("UCP_AGGREGATION_STARTUP_PROOF_ENABLED", "true")
				management = false
			case "missing directory":
				t.Setenv("UCP_AGGREGATION_STARTUP_PROOF_ENABLED", "true")
			case "unreadable directory", "different pod":
				t.Setenv("UCP_AGGREGATION_STARTUP_PROOF_ENABLED", "true")
				service.options.Config.Server.TLSCertificateDirectory = t.TempDir()
				if scenario == "different pod" {
					t.Setenv("UCP_MTLS_STARTUP_PROOF_ENABLED", "true")
					t.Setenv("UCP_MTLS_POD_UID", uuid.NewString())
					t.Setenv("UCP_AGGREGATION_POD_UID", uuid.NewString())
				}
			}
			proof, err := service.PrepareAggregationProof(management)
			require.Nil(t, proof)
			if scenario == "default off" {
				require.NoError(t, err)
				server := &http.Server{Handler: http.NotFoundHandler()}
				source, err := service.configureAggregationServing(server)
				require.NoError(t, err)
				require.Nil(t, source)
				require.Nil(t, server.TLSConfig)
			} else {
				require.Error(t, err)
				require.Nil(t, service.aggregationSource)
			}
		})
	}
}
