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

package helm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	streamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/rest"
)

// Exercise the real client-go SPDY exchange rather than replacing the dialer.
func servePodProofForward(t *testing.T, w http.ResponseWriter, r *http.Request, receiver string, port uint16) {
	t.Helper()
	require.Equal(t, http.MethodPost, r.Method)
	_, err := httpstream.Handshake(r, w, []string{"portforward.k8s.io"})
	require.NoError(t, err)
	connection := streamspdy.NewResponseUpgrader().UpgradeResponse(w, r, func(stream httpstream.Stream, replied <-chan struct{}) error {
		require.Equal(t, strconv.Itoa(int(port)), stream.Headers().Get(corev1.PortHeader))
		if stream.Headers().Get(corev1.StreamType) != corev1.StreamTypeData {
			go func() {
				<-replied
				_ = stream.Close()
			}()
			return nil
		}
		go func() {
			<-replied
			defer stream.Close()
			socket, err := net.DialTimeout("tcp", receiver, 5*time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			defer socket.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = io.Copy(socket, stream)
				_ = socket.Close()
			}()
			_, _ = io.Copy(stream, socket)
			_ = stream.Close()
			<-done
		}()
		return nil
	})
	if connection != nil {
		defer connection.Close()
		<-connection.CloseChan()
	}
}

func TestPodProofPortForward(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"success", "client certificate", "denied", "redirect", "untrusted API", "canceled", "handshake canceled", "handshake deadline", "active canceled", "no upgrade", "wrong protocol"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			target := TLSProofTarget{Namespace: "radius-system", PodName: "ucp-one", Port: 9090}
			var received, redirected atomic.Bool
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Store(true)
				require.Empty(t, r.Header.Get("Authorization"))
				_, err := io.WriteString(w, "pod endpoint")
				require.NoError(t, err)
			}))
			defer receiver.Close()
			redirect := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				redirected.Store(true)
			}))
			defer redirect.Close()
			timeout := 5 * time.Second
			if scenario == "handshake deadline" {
				timeout = 300 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(t.Context(), timeout)
			defer cancel()
			closed := make(chan struct{})
			api := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(closed)
				require.Equal(t, "/api/v1/namespaces/radius-system/pods/ucp-one/portforward", r.URL.Path)
				require.Equal(t, "Bearer observer-api", r.Header.Get("Authorization"))
				if scenario == "client certificate" {
					require.NotEmpty(t, r.TLS.VerifiedChains, "kubeconfig client certificate must authenticate the API tunnel")
				}
				switch scenario {
				case "denied":
					http.Error(w, "private upstream response", http.StatusForbidden)
					return
				case "redirect":
					http.Redirect(w, r, redirect.URL, http.StatusTemporaryRedirect)
					return
				case "no upgrade":
					w.WriteHeader(http.StatusOK)
					return
				case "handshake canceled":
					cancel()
					<-r.Context().Done()
					return
				case "handshake deadline":
					<-r.Context().Done()
					return
				case "wrong protocol":
					_, err := httpstream.Handshake(r, w, []string{"portforward.k8s.io"})
					require.NoError(t, err)
					w.Header().Set(httpstream.HeaderProtocolVersion, "unexpected")
					connection := streamspdy.NewResponseUpgrader().UpgradeResponse(w, r, func(httpstream.Stream, <-chan struct{}) error {
						t.Error("must not open streams for an unnegotiated protocol")
						return nil
					})
					if connection != nil {
						defer connection.Close()
						<-connection.CloseChan()
					}
					return
				}
				servePodProofForward(t, w, r, receiver.Listener.Addr().String(), target.Port)
			}))
			var cert, key []byte
			if scenario == "client certificate" {
				cert, key, _ = testSigningCA(t, nil, nil, nil, nil)
				pool := x509.NewCertPool()
				require.True(t, pool.AppendCertsFromPEM(cert))
				api.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool, MinVersion: tls.VersionTLS12}
			}
			api.StartTLS()
			defer api.Close()
			config := &rest.Config{Host: api.URL, BearerToken: "observer-api",
				TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: api.Certificate().Raw}),
					CertData: cert, KeyData: key}}
			if scenario == "untrusted API" {
				config.CAData = nil
			}
			if scenario == "canceled" {
				cancel()
			}
			origin, stop, err := openPodProofPortForward(ctx, config, target)
			if scenario != "success" && scenario != "active canceled" && scenario != "client certificate" {
				require.Error(t, err)
				require.Empty(t, origin)
				require.Nil(t, stop)
				require.NotContains(t, err.Error(), "private upstream response")
				require.False(t, received.Load())
				require.False(t, redirected.Load(), "API redirects must not receive Kubernetes credentials")
				if strings.Contains(scenario, "canceled") {
					require.True(t, errors.Is(err, context.Canceled))
				}
				if scenario == "handshake deadline" {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
				return
			}
			require.NoError(t, err)
			defer stop()
			require.True(t, strings.HasPrefix(origin, "http://127.0.0.1:"))
			transport := &http.Transport{}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: time.Second}
			response, err := client.Get(origin)
			require.NoError(t, err)
			data, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, "pod endpoint", string(data))
			if scenario == "active canceled" {
				cancel()
			}
			stop()
			stop()
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("port-forward did not close its API connection")
			}
			socket, err := net.DialTimeout("tcp", strings.TrimPrefix(origin, "http://"), time.Second)
			if socket != nil {
				_ = socket.Close()
			}
			require.Error(t, err, "stop must close the local listener")
		})
	}
}
