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
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// TestReloadContext checks that reload errors from a shared Source are logged rather than
// discarded.
func TestReloadContext(t *testing.T) {
	require.NotNil(t, ucplog.FromContextOrDiscard(reloadContext()).GetSink())
}

type recordingTransport struct {
	urls []string
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.urls = append(r.urls, req.URL.String())
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}

// clientOptions returns options for mode that trust caPEM and hold a token for each Radius
// receiver, minted for ucp.
func clientOptions(t *testing.T, caPEM []byte, mode authz.Mode) authz.Options {
	t.Helper()
	return callerOptions(t, caPEM, mode, "ucp")
}

// callerOptions returns options for mode that trust caPEM and hold a token minted for
// caller for each Radius receiver.
func callerOptions(t *testing.T, caPEM []byte, mode authz.Mode, caller string) authz.Options {
	t.Helper()
	// Like the chart, the CA bundle is in its own directory, not in certificateDirectory.
	certDir, caDir, tokenDir := t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(caDir, CAFile), caPEM, 0o600))
	for _, receiver := range []string{"ucp", "applications-rp", "dynamic-rp"} {
		writeToken(t, tokenDir, receiver, mintToken(t, caller, receiver))
	}
	return authz.Options{Mode: mode, CertificateDirectory: certDir, CADirectory: caDir, TokenDirectory: tokenDir, Namespace: testNamespace}
}

func TestNewUpgradeTransport_Off(t *testing.T) {
	plain := &recordingTransport{}
	rt, err := NewUpgradeTransport(authz.Options{Mode: authz.ModeOff}, plain, []string{"applications-rp"})
	require.NoError(t, err)
	require.Same(t, plain, rt)
}

func TestNewClientTransport_ConfigErrors(t *testing.T) {
	ca := newCA(t)
	valid := clientOptions(t, ca.pem, authz.ModeEnforce)

	tests := []struct {
		name     string
		modify   func(o *authz.Options)
		receiver string
		want     string
	}{
		{name: "missing certificate directory", modify: func(o *authz.Options) { o.CertificateDirectory = "" }, want: "requires certificateDirectory, tokenDirectory, and namespace"},
		{name: "missing token directory", modify: func(o *authz.Options) { o.TokenDirectory = "" }, want: "requires certificateDirectory, tokenDirectory, and namespace"},
		{name: "missing namespace", modify: func(o *authz.Options) { o.Namespace = "" }, want: "requires certificateDirectory, tokenDirectory, and namespace"},
		{name: "missing receiver", receiver: "-", want: "receiver is required"},
		{name: "no CA bundle", modify: func(o *authz.Options) { o.CertificateDirectory, o.CADirectory = t.TempDir(), "" }, want: "failed to load CA bundle"},
		{name: "no CA bundle in CA directory", modify: func(o *authz.Options) { o.CADirectory = t.TempDir() }, want: "failed to load CA bundle"},
		{name: "no token for receiver", receiver: "bicep-de", want: "failed to read ServiceAccount token"},
		{name: "empty token", modify: func(o *authz.Options) {
			o.TokenDirectory = t.TempDir()
			writeToken(t, o.TokenDirectory, "ucp", "")
		}, want: "is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := valid
			if tt.modify != nil {
				tt.modify(&options)
			}
			receiver := "ucp"
			if tt.receiver == "-" {
				receiver = ""
			} else if tt.receiver != "" {
				receiver = tt.receiver
			}
			_, err := NewClientTransport(options, receiver)
			require.ErrorContains(t, err, tt.want)
		})
	}

	_, err := NewUpgradeTransport(valid, &recordingTransport{}, []string{"bicep-de"})
	require.ErrorContains(t, err, "failed to read ServiceAccount token")
}

func TestUpgradeTransport_Routing(t *testing.T) {
	ca := newCA(t)
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			plain, secure := &recordingTransport{}, &recordingTransport{}
			rt, err := NewUpgradeTransport(clientOptions(t, ca.pem, mode), plain, []string{"applications-rp", "dynamic-rp"})
			require.NoError(t, err)
			for host := range rt.(*upgradeTransport).secure {
				rt.(*upgradeTransport).secure[host] = secure
			}

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

// TestUpgradeTransport_SendsToken sends a plain http URL through the upgrade transport to an
// enforcing applications-rp that only allows ucp.
func TestUpgradeTransport_SendsToken(t *testing.T) {
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)

	tests := []struct {
		caller string
		want   int
	}{
		{caller: "ucp", want: http.StatusOK},
		{caller: "controller", want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.caller, func(t *testing.T) {
			rt, err := NewUpgradeTransport(callerOptions(t, ca.pem, authz.ModeEnforce, tt.caller), http.DefaultTransport, []string{"applications-rp"})
			require.NoError(t, err)
			dialTo(t, rt, "applications-rp.radius-system.svc", srv.url)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://applications-rp.radius-system.svc:5443/", nil)
			require.NoError(t, err)
			// A caller-supplied Authorization header is replaced.
			req.Header.Set("Authorization", "Bearer "+mintToken(t, "ucp", "applications-rp")+"x")
			resp, err := (&http.Client{Transport: rt}).Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.Equal(t, tt.want, resp.StatusCode)
			if tt.want == http.StatusOK {
				require.Equal(t, "ucp.radius-system.svc", (<-srv.seen).identity.String())
			}
		})
	}
}

// TestUpgradeTransport_RejectsWrongServer sends a request for dynamic-rp to applications-rp.
// The client must reject the server's SPIFFE ID during the handshake and send no token.
func TestUpgradeTransport_RejectsWrongServer(t *testing.T) {
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeDryRun)

	rt, err := NewUpgradeTransport(clientOptions(t, ca.pem, authz.ModeEnforce), http.DefaultTransport, []string{"dynamic-rp"})
	require.NoError(t, err)
	dialTo(t, rt, "dynamic-rp.radius-system.svc", srv.url)

	_, err = (&http.Client{Transport: rt}).Get("http://dynamic-rp.radius-system.svc:8082/")
	requireCode(t, err, authz.CodePeerCertificateInvalid)
	require.ErrorContains(t, err, "is not \""+spiffeID("dynamic-rp")+"\"")
	// In dryRun the server would serve and log a request without a token, so an empty log
	// and no served request show that the client sent nothing.
	require.Empty(t, srv.seen)
	require.Empty(t, srv.logs.All())
}

// TestClientTransport_NoTokenWithoutVerifiedServer checks that a token is never written to a
// connection whose server failed verification, using a raw TLS listener that records every
// byte the client sends after the handshake.
func TestClientTransport_NoTokenWithoutVerifiedServer(t *testing.T) {
	ca := newCA(t)
	for name, cert := range map[string]testCert{
		"wrong SPIFFE ID": ca.issue(t, certOptions{uris: spiffeURIs("controller")}),
		"wrong CA":        newCA(t).issue(t, certOptions{uris: spiffeURIs("applications-rp")}),
		"DNS SAN only":    ca.issue(t, certOptions{dnsNames: []string{"applications-rp.radius-system.svc"}}),
		"mixed DNS SAN":   ca.issue(t, certOptions{uris: spiffeURIs("applications-rp"), dnsNames: []string{"applications-rp.radius-system.svc"}}),
		"mixed IP SAN":    ca.issue(t, certOptions{uris: spiffeURIs("applications-rp"), ipAddresses: []net.IP{net.ParseIP("127.0.0.1")}}),
		"mixed email SAN": ca.issue(t, certOptions{uris: spiffeURIs("applications-rp"), emails: []string{"service@example.test"}}),
	} {
		t.Run(name, func(t *testing.T) {
			ln, err := tls.Listen("tcp", "127.0.0.1:0", staticServer(t, cert))
			require.NoError(t, err)
			t.Cleanup(func() { _ = ln.Close() })
			received := make(chan []byte, 1)
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				data, _ := io.ReadAll(conn)
				received <- data
			}()

			options := clientOptions(t, ca.pem, authz.ModeEnforce)
			transport, err := newClientTransport(options, "applications-rp")
			require.NoError(t, err)
			t.Cleanup(transport.CloseIdleConnections)

			_, err = (&http.Client{Transport: transport}).Get("https://" + ln.Addr().String())
			requireCode(t, err, authz.CodePeerCertificateInvalid)
			require.Empty(t, <-received)
		})
	}
}

func TestClientTransport_RefusesPlainHTTP(t *testing.T) {
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeDryRun)
	transport, err := newClientTransport(clientOptions(t, ca.pem, authz.ModeEnforce), "applications-rp")
	require.NoError(t, err)

	_, err = (&http.Client{Transport: transport}).Get("http://" + srv.url)
	require.ErrorContains(t, err, "must use https")
	require.Empty(t, srv.seen)
}

// TestClientTransport_TokenRotation replaces the projected token, as the kubelet does, and
// checks that the next request sends the new token.
func TestClientTransport_TokenRotation(t *testing.T) {
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)
	options := callerOptions(t, ca.pem, authz.ModeEnforce, "controller")
	transport, err := newClientTransport(options, "applications-rp")
	require.NoError(t, err)
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.url)
	}
	t.Cleanup(transport.CloseIdleConnections)
	c := &http.Client{Transport: transport}
	url := "https://applications-rp.radius-system.svc:5443/"

	require.Equal(t, http.StatusForbidden, send(t, c, url, ""))

	path := filepath.Join(options.TokenDirectory, "applications-rp", TokenFile)
	writeToken(t, options.TokenDirectory, "applications-rp", mintToken(t, "ucp", "applications-rp"))
	// Make the change visible even on file systems with coarse timestamps.
	later := time.Now().Add(time.Minute)
	require.NoError(t, os.Chtimes(path, later, later))

	require.Equal(t, http.StatusOK, send(t, c, url, ""))
	require.Equal(t, "ucp.radius-system.svc", (<-srv.seen).identity.String())
}

type fakeFileInfo struct {
	fs.FileInfo
	modTime time.Time
	size    int64
}

func (f fakeFileInfo) ModTime() time.Time { return f.modTime }
func (f fakeFileInfo) Size() int64        { return f.size }

func TestTokenFile(t *testing.T) {
	now := time.Now()
	info := fakeFileInfo{modTime: now, size: 6}
	content := "first\n"
	var reads, statErrs int
	var statErr, readErr error
	f := newTokenFile("token")
	f.now = func() time.Time { return now }
	f.stat = func(string) (os.FileInfo, error) {
		if statErr != nil {
			statErrs++
			return nil, statErr
		}
		return info, nil
	}
	f.readFile = func(string) ([]byte, error) {
		reads++
		return []byte(content), readErr
	}

	tok, err := f.Token()
	require.NoError(t, err)
	require.Equal(t, "first", tok)
	require.Equal(t, 1, reads)

	// Unchanged files are served from the cache.
	tok, err = f.Token()
	require.NoError(t, err)
	require.Equal(t, "first", tok)
	require.Equal(t, 1, reads)

	// A new modification time is read.
	content = "second"
	info.modTime = now.Add(time.Second)
	tok, err = f.Token()
	require.NoError(t, err)
	require.Equal(t, "second", tok)
	require.Equal(t, 2, reads)

	// A new size is read.
	content = "third!"
	info.size = 7
	tok, err = f.Token()
	require.NoError(t, err)
	require.Equal(t, "third!", tok)
	require.Equal(t, 3, reads)

	// An unchanged file is read again after tokenMaxAge.
	content = "fourth"
	now = now.Add(tokenMaxAge)
	tok, err = f.Token()
	require.NoError(t, err)
	require.Equal(t, "fourth", tok)
	require.Equal(t, 4, reads)

	// A missing or empty file is an error, not the cached token.
	statErr = fs.ErrNotExist
	_, err = f.Token()
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.Equal(t, 1, statErrs)
	statErr = nil
	content = " \n"
	info.size = 2
	_, err = f.Token()
	require.ErrorContains(t, err, "is empty")
	readErr = errors.New("read failed")
	info.size = 3
	_, err = f.Token()
	require.ErrorContains(t, err, "read failed")
}

// dialTo sends every connection of the transport for host to addr.
func dialTo(t *testing.T, rt http.RoundTripper, host, addr string) {
	t.Helper()
	secure := rt.(*upgradeTransport).secure[host].(*clientTransport)
	secure.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	t.Cleanup(secure.CloseIdleConnections)
}

// TestClientTransport_RotationMidHandshake rotates the client's CA bundle to a new CA while
// the server is handling the client hello. The connection must finish with the bundle it
// started with, and the next connection must use the new bundle.
func TestClientTransport_RotationMidHandshake(t *testing.T) {
	oldCA, newCA := newCA(t), newCA(t)
	options := clientOptions(t, oldCA.pem, authz.ModeEnforce)
	source, err := sharedTrustSource(options.EffectiveCADirectory())
	require.NoError(t, err)

	serverCert := oldCA.issue(t, certOptions{uris: spiffeURIs("applications-rp")}).tlsCertificate(t)
	var rotate sync.Once
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			rotate.Do(func() {
				assert.NoError(t, os.WriteFile(filepath.Join(options.EffectiveCADirectory(), CAFile), newCA.pem, 0o600))
				changed, err := source.Reload()
				assert.NoError(t, err)
				assert.True(t, changed)
			})
			return &serverCert, nil
		},
	})
	require.NoError(t, err)

	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	transport, err := newClientTransport(options, "applications-rp")
	require.NoError(t, err)
	require.Nil(t, transport.Proxy)
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, ln.Addr().String())
	}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}

	require.Equal(t, http.StatusOK, send(t, client, "https://applications-rp.radius-system.svc:5443/", ""))

	// A new connection uses the rotated bundle, which does not trust the old server.
	transport.CloseIdleConnections()
	_, err = client.Get("https://applications-rp.radius-system.svc:5443/")
	require.ErrorContains(t, err, "unknown authority")
}

func expiringCA(t *testing.T, parent *testCA, notAfter time.Time) testCA {
	t.Helper()
	ca := newCA(t)
	template := *ca.cert
	template.NotAfter = notAfter
	if parent == nil {
		parent = &ca
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, parent.cert, ca.key.Public(), parent.key)
	require.NoError(t, err)
	ca.cert, err = x509.ParseCertificate(der)
	require.NoError(t, err)
	ca.pem = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return ca
}

func TestClientTransport_PeerExpiresOnOpenConnection(t *testing.T) {
	for _, http2 := range []bool{false, true} {
		for _, kind := range []string{"leaf", "intermediate", "root"} {
			for _, renewed := range []bool{false, true} {
				t.Run(fmt.Sprintf("http2=%t/%s/renewed=%t", http2, kind, renewed), func(t *testing.T) {
					now := time.Now().UTC().Truncate(time.Second)
					expires := now.Add(10 * time.Minute)
					root := expiringCA(t, nil, now.Add(4*time.Hour))
					if kind == "root" {
						root = expiringCA(t, nil, expires)
					}
					issuer := root
					if kind == "intermediate" {
						issuer = expiringCA(t, &root, expires)
					}
					leafExpiry := now.Add(2 * time.Hour)
					if kind == "leaf" {
						leafExpiry = expires
					}
					first := issuer.issue(t, certOptions{uris: spiffeURIs("applications-rp"), notAfter: leafExpiry}).tlsCertificate(t)
					if kind == "intermediate" {
						first.Certificate = append(first.Certificate, issuer.cert.Raw)
					}
					var served atomic.Pointer[tls.Certificate]
					served.Store(&first)
					streamCanceled := make(chan struct{})
					srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/stream" {
							w.WriteHeader(http.StatusOK)
							w.(http.Flusher).Flush()
							<-r.Context().Done()
							close(streamCanceled)
							return
						}
						fmt.Fprint(w, r.RemoteAddr)
					}))
					srv.EnableHTTP2 = http2
					srv.TLS = &tls.Config{
						MinVersion: tls.VersionTLS12,
						GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
							return served.Load(), nil
						},
					}
					srv.StartTLS()
					t.Cleanup(srv.Close)

					options := clientOptions(t, root.pem, authz.ModeEnforce)
					transport, err := newClientTransport(options, "applications-rp")
					require.NoError(t, err)
					t.Cleanup(transport.CloseIdleConnections)
					// httptest serves its own certificate to clients that send no SNI, so use a
					// host name and dial the test server.
					transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
						return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
					}
					baseURL := "https://applications-rp.radius-system.svc:5443"
					client := &http.Client{Transport: transport}
					request := func() (bool, error) {
						var reused bool
						req, err := http.NewRequestWithContext(httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{
							GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
						}), http.MethodGet, baseURL, nil)
						require.NoError(t, err)
						response, err := client.Do(req)
						if err != nil {
							return reused, err
						}
						defer response.Body.Close()
						_, err = io.Copy(io.Discard, response.Body)
						require.NoError(t, err)
						require.Equal(t, http.StatusOK, response.StatusCode)
						if http2 {
							require.Equal(t, 2, response.ProtoMajor)
						}
						return reused, nil
					}
					reused, err := request()
					require.NoError(t, err)
					require.False(t, reused)
					reused, err = request()
					require.NoError(t, err)
					require.True(t, reused, "keep a real TLS connection open across expiry")

					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					defer cancel()
					var stream *http.Response
					if http2 {
						var streamReused bool
						req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
							GotConn: func(info httptrace.GotConnInfo) { streamReused = info.Reused },
						}), http.MethodGet, baseURL+"/stream", nil)
						require.NoError(t, err)
						stream, err = client.Do(req)
						require.NoError(t, err)
						defer stream.Body.Close()
						require.Equal(t, http.StatusOK, stream.StatusCode)
						require.Equal(t, 2, stream.ProtoMajor)
						require.True(t, streamReused, "hold a stream open on the connection whose certificate will expire")
						select {
						case <-streamCanceled:
							t.Fatal("stream ended before certificate expiry")
						default:
						}
					}

					transport.source.mu.Lock()
					transport.source.now = func() time.Time { return now.Add(30 * time.Minute) }
					transport.source.mu.Unlock()
					if renewed {
						nextRoot := expiringCA(t, nil, now.Add(4*time.Hour))
						next := nextRoot.issue(t, certOptions{uris: spiffeURIs("applications-rp"), notAfter: now.Add(2 * time.Hour)}).tlsCertificate(t)
						served.Store(&next)
						require.NoError(t, os.WriteFile(filepath.Join(options.EffectiveCADirectory(), CAFile), nextRoot.pem, 0o600))
						changed, err := transport.source.Reload()
						require.NoError(t, err)
						require.True(t, changed)
					}
					reused, err = request()
					if renewed {
						require.NoError(t, err)
						require.False(t, reused, "renewal must replace the expired peer connection")
					} else {
						require.ErrorContains(t, err, "certificate has expired")
					}
					if http2 {
						_, err := io.Copy(io.Discard, stream.Body)
						require.Error(t, err, "closing the expired connection must interrupt the active response")
						require.NotErrorIs(t, err, context.DeadlineExceeded)
						select {
						case <-streamCanceled:
						case <-ctx.Done():
							t.Fatal("server did not observe cancellation of the active stream")
						}
					}
				})
			}
		}
	}
}
