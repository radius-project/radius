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
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

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

// clientTransport stops reusing connections outside the validity of the presented client
// certificate or any certificate in the server's verified chains.
//
// The embedded Transport is a template. Requests use current, a clone that is replaced when
// one of its connections expires. Replaced pools are retired until their last tracked
// connection closes.
type clientTransport struct {
	*http.Transport
	source     *Source
	namespace  string
	serverName string

	mu      sync.Mutex
	current *http.Transport
	retired map[*http.Transport]struct{}
	conns   map[*trackedConn]connectionValidity
}

type connectionValidity struct {
	notBefore time.Time
	notAfter  time.Time
}

// trackedConn is a connection that clientTransport stops tracking when it is closed.
type trackedConn struct {
	net.Conn
	transport *clientTransport
	// pool is the connection pool that dialed the connection, or nil for the template.
	pool *http.Transport
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
		Transport:  http.DefaultTransport.(*http.Transport).Clone(),
		source:     source,
		namespace:  options.Namespace,
		serverName: serverName,
		retired:    map[*http.Transport]struct{}{},
		conns:      map[*trackedConn]connectionValidity{},
	}
	// A proxy would tunnel the TLS connection without the client certificate, and
	// internal calls never go through a proxy.
	t.Proxy = nil
	t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return t.dialTLS(ctx, network, addr, nil)
	}
	return t, nil
}

// newPool returns a connection pool cloned from the template whose connections record it.
func (t *clientTransport) newPool() *http.Transport {
	pool := t.Transport.Clone()
	pool.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return t.dialTLS(ctx, network, addr, pool)
	}
	return pool
}

func (t *clientTransport) dialTLS(ctx context.Context, network, addr string, pool *http.Transport) (net.Conn, error) {
	name := t.serverName
	if name == "" {
		name = hostname(addr)
	}

	config, leaf, err := t.source.clientConfig(t.namespace)
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
	tracked := &trackedConn{Conn: raw, transport: t, pool: pool}
	conn := tls.Client(tracked, config)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}

	validity := connectionValidity{notBefore: leaf.NotBefore, notAfter: leaf.NotAfter}
	for _, chain := range conn.ConnectionState().VerifiedChains {
		for _, cert := range chain {
			if cert.NotBefore.After(validity.notBefore) {
				validity.notBefore = cert.NotBefore
			}
			if cert.NotAfter.Before(validity.notAfter) {
				validity.notAfter = cert.NotAfter
			}
		}
	}
	t.mu.Lock()
	t.conns[tracked] = validity
	// A request that started before its pool was retired can still dial on it.
	if pool != nil && pool != t.current {
		t.retired[pool] = struct{}{}
	}
	t.mu.Unlock()
	return conn, nil
}

// RoundTrip closes connections whose client certificate or verified server chain is no
// longer valid, forcing a fresh handshake with current material.
func (t *clientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.transportForRequest().RoundTrip(req)
}

// CloseIdleConnections closes idle connections in the current and retired connection pools.
func (t *clientTransport) CloseIdleConnections() {
	t.mu.Lock()
	pools := t.poolsLocked()
	t.mu.Unlock()
	for _, pool := range pools {
		pool.CloseIdleConnections()
	}
}

func (t *clientTransport) poolsLocked() []*http.Transport {
	pools := make([]*http.Transport, 0, len(t.retired)+1)
	if t.current != nil {
		pools = append(pools, t.current)
	}
	for pool := range t.retired {
		pools = append(pools, pool)
	}
	return pools
}

func (t *clientTransport) transportForRequest() *http.Transport {
	var expired []*trackedConn
	t.mu.Lock()
	now := t.source.now()
	replace := t.current == nil
	for conn, validity := range t.conns {
		if now.Before(validity.notBefore) || now.After(validity.notAfter) {
			expired = append(expired, conn)
			// An expired connection in a retired pool cannot serve this request, so closing
			// it is enough.
			replace = replace || conn.pool == t.current
		}
	}
	if replace {
		// An active HTTP/2 connection stays in its pool until its read loop observes
		// the close. Use a fresh pool so this request cannot select that connection.
		if t.current != nil {
			t.retired[t.current] = struct{}{}
		}
		t.current = t.newPool()
	}
	// Close idle connections in retired pools, including connections that finished their
	// requests since the pool was retired, and forget pools with no connections left.
	var retired []*http.Transport
	if len(t.retired) > 0 {
		retired = t.retiredInUseLocked(now)
	}
	current := t.current
	t.mu.Unlock()

	// Connections in use are closed too, which aborts their in-flight requests: an HTTP/2
	// connection in use would otherwise take new requests.
	for _, conn := range expired {
		_ = conn.Close()
	}
	for _, pool := range retired {
		pool.CloseIdleConnections()
	}
	return current
}

// retiredInUseLocked forgets retired pools without unexpired connections and returns the rest.
func (t *clientTransport) retiredInUseLocked(now time.Time) []*http.Transport {
	live := map[*http.Transport]bool{}
	for conn, validity := range t.conns {
		if !now.Before(validity.notBefore) && !now.After(validity.notAfter) {
			live[conn.pool] = true
		}
	}
	var retired []*http.Transport
	for pool := range t.retired {
		if live[pool] {
			retired = append(retired, pool)
		} else {
			delete(t.retired, pool)
		}
	}
	return retired
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
