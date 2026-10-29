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
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/go-logr/logr"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/radius-project/radius/pkg/authz"
)

var (
	sharedMu      sync.Mutex
	sharedSources = map[string]*Source{}
)

// sharedSource returns the process-wide Source for dir, creating it on first use. Clients
// live for the whole process, so the Source reloads the directory until the process exits.
func sharedSource(dir string) (*Source, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()

	if s, ok := sharedSources[dir]; ok {
		return s, nil
	}

	s, err := NewSource(dir)
	if err != nil {
		return nil, err
	}
	go s.Start(reloadContext(), DefaultPollInterval)
	sharedSources[dir] = s
	return s, nil
}

// reloadContext returns the context for a shared Source's reload loop. Transports are often
// built before the component creates its logger, so the context carries controller-runtime's
// process logger, which every Radius component sets at startup and which forwards to that
// logger once it is set.
func reloadContext() context.Context {
	return logr.NewContext(context.Background(), runtimelog.Log)
}

// NewClientTransport returns an HTTP transport that presents this component's certificate
// and verifies that the server is the Radius component named by the request URL's host.
// It requires options.CertificateDirectory and options.Namespace.
func NewClientTransport(options authz.Options) (http.RoundTripper, error) {
	return newClientTransport(options, "")
}

// clientTransport is an http.Transport that stops using a connection once the certificate
// it presented is outside its validity period. The TLS handshake checks the certificate
// only when a connection is opened, so without this a reused connection would outlive it.
type clientTransport struct {
	*http.Transport
	source *Source

	mu sync.Mutex
	// conns maps each open connection to the certificate it presented.
	conns map[*trackedConn]*x509.Certificate
}

// trackedConn is a connection that clientTransport stops tracking when it is closed.
type trackedConn struct {
	net.Conn
	transport *clientTransport
}

// Close implements net.Conn.
func (c *trackedConn) Close() error {
	c.transport.mu.Lock()
	delete(c.transport.conns, c)
	c.transport.mu.Unlock()
	return c.Conn.Close()
}

// newClientTransport is NewClientTransport with an optional TLS server name that replaces
// the request URL's host.
//
// Each connection reads the certificate and CA bundle once, so the certificate it presents
// and the CA bundle it verifies the server against always come from the same version.
func newClientTransport(options authz.Options, serverName string) (*clientTransport, error) {
	if options.CertificateDirectory == "" || options.Namespace == "" {
		return nil, fmt.Errorf("authorization mode %q requires certificateDirectory and namespace", options.EffectiveMode())
	}

	source, err := sharedSource(options.CertificateDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load mTLS certificate: %w", err)
	}

	t := &clientTransport{
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
		source:    source,
		conns:     map[*trackedConn]*x509.Certificate{},
	}
	// A proxy would tunnel the TLS connection without the client certificate, and
	// internal calls never go through a proxy.
	t.Proxy = nil
	t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		name := serverName
		if name == "" {
			name = hostname(addr)
		}

		config, leaf, err := source.clientConfig(options.Namespace)
		if err != nil {
			return nil, err
		}
		config.ServerName = name
		config.NextProtos = []string{"h2", "http/1.1"}

		raw, err := t.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(ctx, t.TLSHandshakeTimeout)
		defer cancel()
		tracked := &trackedConn{Conn: raw, transport: t}
		conn := tls.Client(tracked, config)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}

		t.mu.Lock()
		t.conns[tracked] = leaf
		t.mu.Unlock()
		return conn, nil
	}
	return t, nil
}

// RoundTrip implements http.RoundTripper. It first closes every connection whose presented
// certificate is outside its validity period, so the request uses a new connection, which
// presents the current certificate or fails with CertificateExpired.
func (t *clientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.closeExpired()
	return t.Transport.RoundTrip(req)
}

func (t *clientTransport) closeExpired() {
	var expired []*trackedConn
	t.mu.Lock()
	for conn, leaf := range t.conns {
		if !t.source.isValid(leaf) {
			expired = append(expired, conn)
		}
	}
	t.mu.Unlock()
	if len(expired) == 0 {
		return
	}

	// Closing idle connections removes them from the pool before this request picks one.
	// Connections in use are closed too, which aborts their in-flight requests: an HTTP/2
	// connection in use would otherwise take new requests.
	t.CloseIdleConnections()
	for _, conn := range expired {
		_ = conn.Close()
	}
}

// NewUpgradeTransport returns plain when the authorization mode is off. Otherwise it returns
// a transport that sends requests for the given components over mTLS, changing an http URL
// to https, and sends all other requests through plain. A component matches by any of its
// DNS names: <component>, <component>.<namespace>, or <component>.<namespace>.svc. The TLS
// server name is always the canonical <component>.<namespace>.svc, the only name that
// component certificates carry.
func NewUpgradeTransport(options authz.Options, plain http.RoundTripper, components []string) (http.RoundTripper, error) {
	if options.EffectiveMode() == authz.ModeOff {
		return plain, nil
	}

	secure := map[string]http.RoundTripper{}
	for _, c := range components {
		transport, err := newClientTransport(options, c+"."+options.Namespace+".svc")
		if err != nil {
			return nil, err
		}

		secure[c] = transport
		secure[c+"."+options.Namespace] = transport
		secure[c+"."+options.Namespace+".svc"] = transport
	}
	return &upgradeTransport{plain: plain, secure: secure}, nil
}

type upgradeTransport struct {
	plain http.RoundTripper
	// secure maps each accepted host name to the mTLS transport for its component.
	secure map[string]http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t *upgradeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	secure, ok := t.secure[hostname(req.URL.Host)]
	if !ok {
		return t.plain.RoundTrip(req)
	}

	if req.URL.Scheme == "http" {
		req = req.Clone(req.Context())
		req.URL.Scheme = "https"
	}
	return secure.RoundTrip(req)
}

func hostname(hostport string) string {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	return hostport
}
