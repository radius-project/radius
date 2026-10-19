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
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/authz"
)

type recordingTransport struct {
	urls []string
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.urls = append(r.urls, req.URL.String())
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}

// componentOptions returns options for mode with a certificate for component from ca.
func componentOptions(t *testing.T, ca testCA, component string, mode authz.Mode) authz.Options {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, ca.issue(t, certOptions{dnsNames: componentSANs(component)}), ca.pem)
	return authz.Options{Mode: mode, CertificateDirectory: dir, Namespace: testNamespace}
}

func TestNewUpgradeTransport_Off(t *testing.T) {
	plain := &recordingTransport{}
	rt, err := NewUpgradeTransport(authz.Options{Mode: authz.ModeOff}, plain, []string{"applications-rp"})
	require.NoError(t, err)
	require.Same(t, plain, rt)
}

func TestNewClientTransport_ConfigErrors(t *testing.T) {
	ca := newCA(t)
	valid := componentOptions(t, ca, "ucp", authz.ModeEnforce)

	tests := []struct {
		name   string
		modify func(o *authz.Options)
		want   string
	}{
		{name: "missing directory", modify: func(o *authz.Options) { o.CertificateDirectory = "" }, want: "requires certificateDirectory and namespace"},
		{name: "missing namespace", modify: func(o *authz.Options) { o.Namespace = "" }, want: "requires certificateDirectory and namespace"},
		{name: "unreadable directory", modify: func(o *authz.Options) { o.CertificateDirectory = t.TempDir() }, want: "failed to load mTLS certificate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := valid
			tt.modify(&options)
			_, err := NewUpgradeTransport(options, &recordingTransport{}, []string{"applications-rp"})
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestUpgradeTransport_Routing(t *testing.T) {
	ca := newCA(t)
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			plain, secure := &recordingTransport{}, &recordingTransport{}
			rt, err := NewUpgradeTransport(componentOptions(t, ca, "ucp", mode), plain, []string{"applications-rp", "dynamic-rp"})
			require.NoError(t, err)
			rt.(*upgradeTransport).secure = secure

			for _, u := range []string{
				"http://applications-rp:5443/a",
				"http://applications-rp.radius-system:5443/a",
				"http://dynamic-rp.radius-system.svc:8082/a",
				"https://dynamic-rp.radius-system:8082/a",
				"http://bicep-de.radius-system:6443/a",
				"http://applications-rp.other:5443/a",
				"https://example.com/a",
			} {
				req, err := http.NewRequest(http.MethodGet, u, nil)
				require.NoError(t, err)
				_, err = rt.RoundTrip(req)
				require.NoError(t, err)
				require.Equalf(t, u, req.URL.String(), "the caller's request must not be modified")
			}

			require.Equal(t, []string{
				"https://applications-rp:5443/a",
				"https://applications-rp.radius-system:5443/a",
				"https://dynamic-rp.radius-system.svc:8082/a",
				"https://dynamic-rp.radius-system:8082/a",
			}, secure.urls)
			require.Equal(t, []string{
				"http://bicep-de.radius-system:6443/a",
				"http://applications-rp.other:5443/a",
				"https://example.com/a",
			}, plain.urls)
		})
	}
}

// TestUpgradeTransport_PresentsCertificate sends a plain http URL through the upgrade
// transport to an enforcing applications-rp that only allows ucp.
func TestUpgradeTransport_PresentsCertificate(t *testing.T) {
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)

	tests := []struct {
		component string
		want      int
	}{
		{component: "ucp", want: http.StatusOK},
		{component: "controller", want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.component, func(t *testing.T) {
			rt, err := NewUpgradeTransport(componentOptions(t, ca, tt.component, authz.ModeEnforce), http.DefaultTransport, []string{"applications-rp"})
			require.NoError(t, err)
			secure := rt.(*upgradeTransport).secure.(*http.Transport)
			secure.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, srv.url)
			}
			t.Cleanup(secure.CloseIdleConnections)

			status, err := get(&http.Client{Transport: rt}, "http://"+appsRPServerName+":5443/")
			require.NoError(t, err)
			require.Equal(t, tt.want, status)
		})
	}
}

func TestUpgradeTransport_RejectsWrongServer(t *testing.T) {
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)

	rt, err := NewUpgradeTransport(componentOptions(t, ca, "ucp", authz.ModeEnforce), http.DefaultTransport, []string{"dynamic-rp"})
	require.NoError(t, err)
	secure := rt.(*upgradeTransport).secure.(*http.Transport)
	secure.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.url)
	}
	t.Cleanup(secure.CloseIdleConnections)

	// The server is applications-rp, so a request for dynamic-rp must fail verification.
	_, err = get(&http.Client{Transport: rt}, "http://dynamic-rp."+testNamespace+".svc:8082/")
	require.ErrorContains(t, err, "not dynamic-rp."+testNamespace+".svc")
}

// TestClientTransport_RotationMidHandshake rotates the client's certificate and CA bundle
// to a new CA while the server is handling the client hello. The connection must finish
// with the material it started with, and the next connection must use the new material.
func TestClientTransport_RotationMidHandshake(t *testing.T) {
	oldCA, newCA := newCA(t), newCA(t)
	options := componentOptions(t, oldCA, "ucp", authz.ModeEnforce)
	oldClient, err := tls.LoadX509KeyPair(filepath.Join(options.CertificateDirectory, CertFile), filepath.Join(options.CertificateDirectory, KeyFile))
	require.NoError(t, err)
	source, err := sharedSource(options.CertificateDirectory)
	require.NoError(t, err)

	serverCert := oldCA.issue(t, certOptions{dnsNames: componentSANs("applications-rp")}).tlsCertificate(t)
	clientCAs := x509.NewCertPool()
	clientCAs.AddCert(oldCA.cert)

	var rotate sync.Once
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  clientCAs,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			rotate.Do(func() {
				writeFiles(t, options.CertificateDirectory, newCA.issue(t, certOptions{dnsNames: componentSANs("ucp")}), newCA.pem)
				changed, err := source.Reload()
				assert.NoError(t, err)
				assert.True(t, changed)
			})
			return &serverCert, nil
		},
	})
	require.NoError(t, err)

	presented := make(chan []byte, 1)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented <- r.TLS.PeerCertificates[0].Raw
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	transport, err := NewClientTransport(options)
	require.NoError(t, err)
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, ln.Addr().String())
	}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}

	status, err := get(client, "https://"+appsRPServerName+":5443/")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, oldClient.Certificate[0], <-presented)

	// A new connection uses the rotated material, which does not trust the old server.
	transport.CloseIdleConnections()
	_, err = get(client, "https://"+appsRPServerName+":5443/")
	require.ErrorContains(t, err, "unknown authority")
}
