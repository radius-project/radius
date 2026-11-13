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

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

type servingConnectionKey struct{}

func (s *Source) retireServingGenerations(current *material) {
	s.serving.Range(func(key, value any) bool {
		handshake := value.(*material)
		if _, err := handshake.cert.Leaf.Verify(x509.VerifyOptions{Roots: current.pool,
			Intermediates: handshake.intermediates, CurrentTime: s.now(),
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			s.serving.Delete(key)
		}
		return true
	})
}

// GuardServingGeneration checks the actual handshake's own chain on every request,
// including pooled HTTP/1.1 and HTTP/2. Install before starting the server.
func (s *Source) GuardServingGeneration(server *http.Server) {
	s.guardServing.Store(true)
	priorContext, priorState := server.ConnContext, server.ConnState
	server.ConnContext = func(ctx context.Context, conn net.Conn) context.Context {
		if priorContext != nil {
			ctx = priorContext(ctx, conn)
		}
		if conn, ok := conn.(*tls.Conn); ok {
			return context.WithValue(ctx, servingConnectionKey{}, conn.NetConn())
		}
		return ctx
	}
	server.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateClosed || state == http.StateHijacked {
			if conn, ok := conn.(*tls.Conn); ok {
				s.serving.Delete(conn.NetConn())
			}
		}
		if priorState != nil {
			priorState(conn, state)
		}
	}
	next := server.Handler
	if next == nil {
		next = http.DefaultServeMux
	}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			next.ServeHTTP(w, r)
			return
		}
		current, err := s.load()
		if err != nil {
			s.serving.Delete(r.Context().Value(servingConnectionKey{}))
			ucplog.FromContextOrDiscard(r.Context()).Error(err, "Serving trust unavailable")
			http.Error(w, "TLS serving generation unavailable", http.StatusServiceUnavailable)
			return
		}
		value, ok := s.serving.Load(r.Context().Value(servingConnectionKey{}))
		if !ok {
			http.Error(w, "TLS handshake generation unavailable", http.StatusServiceUnavailable)
			return
		}
		handshake := value.(*material)
		if handshake.epoch != current.epoch {
			http.Error(w, "TLS serving generation invalidated", http.StatusServiceUnavailable)
			return
		}
		if err := s.checkMaterial(handshake); err != nil {
			http.Error(w, "TLS serving generation expired", http.StatusServiceUnavailable)
			return
		}

		if _, err := handshake.cert.Leaf.Verify(x509.VerifyOptions{Roots: current.pool,
			Intermediates: handshake.intermediates, CurrentTime: s.now(),
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			http.Error(w, "TLS serving generation retired", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}
