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
	go s.Start(context.Background(), DefaultPollInterval)
	sharedSources[dir] = s
	return s, nil
}

// NewClientTransport returns an HTTP transport that presents this component's certificate
// and verifies that the server is the Radius component named by the request URL's host.
// It requires options.CertificateDirectory and options.Namespace.
//
// Each connection reads the certificate and CA bundle once, so the certificate it presents
// and the CA bundle it verifies the server against always come from the same version.
func NewClientTransport(options authz.Options) (*http.Transport, error) {
	if options.CertificateDirectory == "" || options.Namespace == "" {
		return nil, fmt.Errorf("authorization mode %q requires certificateDirectory and namespace", options.EffectiveMode())
	}

	source, err := sharedSource(options.CertificateDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load mTLS certificate: %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		name := hostname(addr)

		config, err := source.ClientConfig(options.Namespace)
		if err != nil {
			return nil, err
		}
		config.ServerName = name
		config.NextProtos = []string{"h2", "http/1.1"}

		raw, err := transport.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(ctx, transport.TLSHandshakeTimeout)
		defer cancel()
		conn := tls.Client(raw, config)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}
		return conn, nil
	}
	return transport, nil
}

// NewUpgradeTransport returns plain when the authorization mode is off. Otherwise it returns
// a transport that sends requests for the given components over mTLS, changing an http URL
// to https, and sends all other requests through plain. A component matches by any of its
// DNS names: <component>, <component>.<namespace>, or <component>.<namespace>.svc.
func NewUpgradeTransport(options authz.Options, plain http.RoundTripper, components []string) (http.RoundTripper, error) {
	if options.EffectiveMode() == authz.ModeOff {
		return plain, nil
	}

	secure, err := NewClientTransport(options)
	if err != nil {
		return nil, err
	}

	hosts := map[string]bool{}
	for _, c := range components {
		hosts[c] = true
		hosts[c+"."+options.Namespace] = true
		hosts[c+"."+options.Namespace+".svc"] = true
	}
	return &upgradeTransport{plain: plain, secure: secure, hosts: hosts}, nil
}

type upgradeTransport struct {
	plain  http.RoundTripper
	secure http.RoundTripper
	hosts  map[string]bool
}

// RoundTrip implements http.RoundTripper.
func (t *upgradeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.hosts[hostname(req.URL.Host)] {
		return t.plain.RoundTrip(req)
	}

	if req.URL.Scheme == "http" {
		req = req.Clone(req.Context())
		req.URL.Scheme = "https"
	}
	return t.secure.RoundTrip(req)
}

func hostname(hostport string) string {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	return hostport
}
