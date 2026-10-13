/*
Copyright 2023 The Radius Authors.

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

// Package registryproxy forwards a Pod-loopback TCP port to the Radius in-cluster
// OCI registry.
//
// Node container runtimes allow plain-HTTP pulls only from localhost, so images
// built by Radius.Compute/containerImages are referenced as localhost:<nodePort>/...
// and pulled through the registry's NodePort. The BuildKit sidecar pushes from inside
// the dynamic-rp Pod, where localhost is the Pod's loopback. This proxy makes the same
// reference reachable from the Pod by forwarding the loopback port to the registry
// Service, so push and pull use an identical image reference.
package registryproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const dialTimeout = 10 * time.Second

// Options configures the registry proxy.
type Options struct {
	// Enabled starts the proxy when true.
	Enabled bool `yaml:"enabled,omitempty"`

	// ListenAddress is the loopback address to accept connections on, such as 127.0.0.1:31500.
	ListenAddress string `yaml:"listenAddress,omitempty"`

	// TargetAddress is the registry Service address to forward to, such as
	// radius-registry.radius-system.svc:5000.
	TargetAddress string `yaml:"targetAddress,omitempty"`
}

// Service is a hosting.Service that runs the registry proxy.
type Service struct {
	Options *Options

	// listening, when set, receives the bound listener address once the proxy is accepting connections.
	listening chan<- net.Addr
}

// Name returns the name of the registry proxy service.
func (s *Service) Name() string {
	return "registry proxy"
}

// Run accepts connections on the listen address and forwards each one to the target address
// until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	if s.Options == nil || s.Options.ListenAddress == "" || s.Options.TargetAddress == "" {
		return errors.New("registry proxy requires listenAddress and targetAddress")
	}

	logger := ucplog.FromContextOrDiscard(ctx)

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", s.Options.ListenAddress)
	if err != nil {
		return fmt.Errorf("registry proxy failed to listen on %s: %w", s.Options.ListenAddress, err)
	}

	stopListener := context.AfterFunc(ctx, func() {
		_ = listener.Close()
	})
	defer stopListener()

	logger.Info(fmt.Sprintf("registry proxy forwarding %s to %s", listener.Addr(), s.Options.TargetAddress))
	if s.listening != nil {
		s.listening <- listener.Addr()
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			_ = listener.Close()
			return fmt.Errorf("registry proxy failed to accept a connection: %w", err)
		}

		wg.Go(func() {
			s.forward(ctx, conn)
		})
	}
}

func (s *Service) forward(ctx context.Context, client net.Conn) {
	logger := ucplog.FromContextOrDiscard(ctx)
	defer client.Close()

	dialer := net.Dialer{Timeout: dialTimeout}
	upstream, err := dialer.DialContext(ctx, "tcp", s.Options.TargetAddress)
	if err != nil {
		logger.Error(err, "registry proxy failed to reach the registry", "target", s.Options.TargetAddress)
		return
	}
	defer upstream.Close()

	// Close both sides when the service stops so the copies below return.
	stop := context.AfterFunc(ctx, func() {
		_ = client.Close()
		_ = upstream.Close()
	})
	defer stop()

	var wg sync.WaitGroup
	wg.Go(func() { pipe(upstream, client) })
	wg.Go(func() { pipe(client, upstream) })
	wg.Wait()
}

// pipe copies src to dst, then half-closes dst for writing so the peer sees EOF.
func pipe(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if tcp, ok := dst.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
		return
	}
	_ = dst.Close()
}
