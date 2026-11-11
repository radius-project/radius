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
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-logr/logr"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/radius-project/radius/pkg/authz"
)

const (
	// TokenFile is the file name of a projected ServiceAccount token under
	// <tokenDirectory>/<receiver>/.
	TokenFile = "token"

	// tokenMaxAge is how long a cached token is used before it is read again, even if the
	// file looks unchanged.
	tokenMaxAge = time.Minute
)

var (
	sharedMu      sync.Mutex
	sharedSources = map[string]*Source{}
)

// sharedTrustSource returns the process-wide trust-only Source for dir, creating it on first
// use. Clients live for the whole process, so the Source reloads the directory until the
// process exits.
func sharedTrustSource(dir string) (*Source, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()

	if s, ok := sharedSources[dir]; ok {
		return s, nil
	}

	s, err := NewTrustSource(dir)
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

// NewClientTransport returns an HTTP transport for calls to the Radius component receiver,
// such as "ucp". It presents no client certificate. It accepts only a server whose
// certificate chains to the Radius CA bundle in options.EffectiveCADirectory() and carries the
// receiver's SPIFFE ID, and only then sends the projected ServiceAccount token read from
// <options.TokenDirectory>/<receiver>/token as "Authorization: Bearer". It replaces any
// Authorization header on the request and refuses non-https URLs.
func NewClientTransport(options authz.Options, receiver string) (http.RoundTripper, error) {
	return newClientTransport(options, receiver)
}

// clientTransport attaches the receiver's token and stops reusing connections outside the
// validity of any certificate in the server's verified chain.
type clientTransport struct {
	*http.Transport
	source *Source
	token  *tokenFile

	mu    sync.Mutex
	conns map[*trackedConn]connectionValidity
}

type connectionValidity struct {
	notBefore time.Time
	notAfter  time.Time
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

func newClientTransport(options authz.Options, receiver string) (*clientTransport, error) {
	mode := options.EffectiveMode()
	if options.CertificateDirectory == "" || options.TokenDirectory == "" || options.Namespace == "" {
		return nil, fmt.Errorf("authorization mode %q requires certificateDirectory, tokenDirectory, and namespace", mode)
	}
	if receiver == "" {
		return nil, errors.New("receiver is required")
	}

	source, err := sharedTrustSource(options.EffectiveCADirectory())
	if err != nil {
		return nil, fmt.Errorf("failed to load CA bundle: %w", err)
	}

	token := newTokenFile(filepath.Join(options.TokenDirectory, receiver, TokenFile))
	if _, err := token.Token(); err != nil {
		return nil, err
	}

	trustDomain := options.TrustDomain
	if trustDomain == "" {
		trustDomain = DefaultTrustDomain
	}
	spiffeID := SPIFFEID(trustDomain, options.Namespace, receiver)

	t := &clientTransport{
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
		source:    source,
		token:     token,
		conns:     map[*trackedConn]connectionValidity{},
	}
	// A proxy would see the token's destination and could hold the connection open, and
	// internal calls never go through a proxy.
	t.Proxy = nil
	t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		var validity connectionValidity
		config, err := source.clientConfig(spiffeID, func(chain []*x509.Certificate) {
			validity = chainValidity(chain)
		})
		if err != nil {
			return nil, err
		}
		config.ServerName = hostname(addr)
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
		t.conns[tracked] = validity
		t.mu.Unlock()
		return conn, nil
	}
	return t, nil
}

// chainValidity returns the period in which every certificate in chain is valid.
func chainValidity(chain []*x509.Certificate) connectionValidity {
	var v connectionValidity
	for i, cert := range chain {
		if i == 0 || cert.NotBefore.After(v.notBefore) {
			v.notBefore = cert.NotBefore
		}
		if i == 0 || cert.NotAfter.Before(v.notAfter) {
			v.notAfter = cert.NotAfter
		}
	}
	return v
}

// RoundTrip sends req with the receiver's token. The token is written only to connections
// whose server passed verification in DialTLSContext, so it is never sent to a server that
// failed verification. Connections whose verified server chain is no longer valid are closed
// first, forcing a fresh handshake with the current CA bundle.
func (t *clientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("refusing to send a ServiceAccount token over %q; the URL must use https", req.URL.Scheme)
	}
	token, err := t.token.Token()
	if err != nil {
		return nil, err
	}
	t.closeExpired()

	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+token)
	return t.Transport.RoundTrip(req)
}

func (t *clientTransport) closeExpired() {
	var expired []*trackedConn
	t.mu.Lock()
	now := t.source.now()
	for conn, validity := range t.conns {
		if now.Before(validity.notBefore) || now.After(validity.notAfter) {
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

// tokenFile caches a projected ServiceAccount token. The kubelet rotates the token by
// replacing the file, so Token reads it again when its modification time or size changes,
// or when the cached copy is older than tokenMaxAge.
type tokenFile struct {
	path     string
	now      func() time.Time
	stat     func(string) (os.FileInfo, error)
	readFile func(string) ([]byte, error)

	mu       sync.Mutex
	token    string
	modTime  time.Time
	size     int64
	loadedAt time.Time
}

func newTokenFile(path string) *tokenFile {
	return &tokenFile{path: path, now: time.Now, stat: os.Stat, readFile: os.ReadFile}
}

// Token returns the current token.
func (f *tokenFile) Token() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	info, err := f.stat(f.path)
	if err != nil {
		return "", fmt.Errorf("failed to read ServiceAccount token: %w", err)
	}
	now := f.now()
	if f.token != "" && info.ModTime().Equal(f.modTime) && info.Size() == f.size && now.Sub(f.loadedAt) < tokenMaxAge {
		return f.token, nil
	}

	data, err := f.readFile(f.path)
	if err != nil {
		return "", fmt.Errorf("failed to read ServiceAccount token: %w", err)
	}
	token := string(bytes.TrimSpace(data))
	if token == "" {
		return "", fmt.Errorf("ServiceAccount token file %s is empty", f.path)
	}

	f.token = token
	f.modTime = info.ModTime()
	f.size = info.Size()
	f.loadedAt = now
	return token, nil
}

// NewUpgradeTransport returns plain when the authorization mode is off. Otherwise it returns
// a transport that sends requests for the given components with NewClientTransport, changing
// an http URL to https, and sends all other requests through plain. A component matches by any
// of its DNS names: <component>, <component>.<namespace>, or <component>.<namespace>.svc.
func NewUpgradeTransport(options authz.Options, plain http.RoundTripper, components []string) (http.RoundTripper, error) {
	if options.EffectiveMode() == authz.ModeOff {
		return plain, nil
	}

	secure := map[string]http.RoundTripper{}
	for _, c := range components {
		transport, err := newClientTransport(options, c)
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
	// secure maps each accepted host name to the transport for its component.
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
