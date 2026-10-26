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
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/go-logr/logr"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// LogFieldReason is the log key for why a peer certificate check failed.
	LogFieldReason = "authzReason"

	// LogFieldPeerAddress is the log key for the remote address of the connection.
	LogFieldPeerAddress = "authzPeerAddress"

	// LogFieldCaller is the log key for the verified calling component.
	LogFieldCaller = "authzCaller"

	// handshakeTimeout bounds how long a connection may take to send its first byte
	// and complete the TLS handshake.
	handshakeTimeout = 10 * time.Second

	// tlsHandshakeRecord is the first byte of a TLS handshake record.
	tlsHandshakeRecord = 0x16
)

// ListenAndServe serves srv according to the authorization mode in options.
//
// In off mode it calls srv.ListenAndServe, so behavior is unchanged.
//
// In dryRun mode the listener accepts both plain HTTP and TLS on the same port. TLS
// connections are asked for a client certificate, which is verified after the handshake.
// Missing or invalid certificates and callers that are not in allowedCallers are logged
// as would-deny, and every request is still served.
//
// In enforce mode the listener only accepts TLS with a verified client certificate.
// Plain HTTP and failed handshakes are closed and logged as denied, and callers that are
// not in allowedCallers get an AuthorizationFailed response.
//
// An empty allowedCallers accepts any Radius component. ListenAndServe changes srv's
// Handler and ConnContext when the mode is not off.
func ListenAndServe(ctx context.Context, srv *http.Server, options authz.Options, allowedCallers []string) error {
	mode := options.EffectiveMode()
	if mode == authz.ModeOff {
		return srv.ListenAndServe()
	}

	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return Serve(ctx, srv, ln, options, allowedCallers)
}

// Serve is like ListenAndServe but serves on an existing listener. It closes ln when
// it returns. In off mode it calls srv.Serve(ln).
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, options authz.Options, allowedCallers []string) error {
	mode := options.EffectiveMode()
	if mode == authz.ModeOff {
		return srv.Serve(ln)
	}

	if options.CertificateDirectory == "" || options.Namespace == "" {
		_ = ln.Close()
		return fmt.Errorf("authorization mode %q requires certificateDirectory and namespace", mode)
	}

	source, err := NewSource(options.CertificateDirectory)
	if err != nil {
		_ = ln.Close()
		return fmt.Errorf("failed to load mTLS certificate: %w", err)
	}

	sourceCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go source.Start(sourceCtx, DefaultPollInterval)

	s := &server{source: source, mode: mode, namespace: options.Namespace, allowedCallers: allowedCallers}

	srv.Handler = s.authorizeCaller(srv.Handler)
	next := srv.ConnContext
	srv.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		if next != nil {
			ctx = next(ctx, c)
		}
		return s.connContext(ctx, c)
	}

	var tlsConfig *tls.Config
	if mode == authz.ModeEnforce {
		tlsConfig = source.ServerConfig(options.Namespace)
	} else {
		tlsConfig = source.requestClientCertConfig()
	}

	return srv.Serve(newListener(ctx, ln, tlsConfig, mode))
}

// requestClientCertConfig returns a server config that asks for, but does not require or
// verify, a client certificate. connContext verifies it after the handshake.
func (s *Source) requestClientCertConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return s.certificate()
		},
		ClientAuth: tls.RequestClientCert,
	}
}

type server struct {
	source         *Source
	mode           authz.Mode
	namespace      string
	allowedCallers []string
}

// connContext adds the verified peer identity to the connection's context. In dryRun
// mode it also verifies the client certificate and logs a would-deny when it fails.
func (s *server) connContext(ctx context.Context, c net.Conn) context.Context {
	tlsConn, ok := c.(*tls.Conn)
	if !ok {
		// Plain HTTP was already logged by the listener.
		return ctx
	}

	state := tlsConn.ConnectionState()
	var id Identity
	var err error
	if s.mode == authz.ModeEnforce {
		id, err = identityFromChains(state.VerifiedChains, s.namespace)
	} else {
		id, err = s.source.verifyClient(state.PeerCertificates, s.namespace)
	}
	if err != nil {
		logPeerFailure(ctx, s.mode, c.RemoteAddr(), err)
		return ctx
	}
	return WithIdentity(ctx, id)
}

// authorizeCaller checks that the verified caller is one of the allowed components.
func (s *server) authorizeCaller(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		decision := authz.Decision{Allowed: true, Action: r.Method, Target: r.URL.Path}

		id, ok := IdentityFromContext(ctx)
		switch {
		case !ok:
			// The connection failure was already logged. Enforce mode never gets here
			// because the handshake requires a verified identity.
			if s.mode == authz.ModeEnforce {
				decision.Allowed = false
				decision.Code = v1.CodeInvalidAuthenticationInfo
			}
		case len(s.allowedCallers) > 0 && !slices.Contains(s.allowedCallers, id.Component):
			decision.Allowed = false
			decision.Code = authz.CodeAuthorizationFailed
			ctx = logr.NewContext(ctx, ucplog.FromContextOrDiscard(ctx).WithValues(LogFieldCaller, id.String()))
		}

		if err := authz.Apply(ctx, s.mode, decision); err != nil {
			writeDenied(ctx, w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeDenied(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	authz.WriteDenied(ctx, w, r, err)
}

// logPeerFailure logs a failed peer certificate check with the authorization log fields.
// Failures that are not an *Error, such as chain verification errors from crypto/tls, are
// logged as PeerCertificateInvalid.
func logPeerFailure(ctx context.Context, mode authz.Mode, remote net.Addr, err error) {
	code := authz.CodePeerCertificateInvalid
	reason := err.Error()
	var mErr *Error
	if errors.As(err, &mErr) {
		code = mErr.Code
		reason = mErr.Error()
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues(LogFieldReason, reason, LogFieldPeerAddress, remote.String())
	_ = authz.Apply(logr.NewContext(ctx, logger), mode, authz.Decision{Code: code})
}

// listener classifies each accepted connection as TLS or plain HTTP by its first byte,
// completes TLS handshakes, and logs failures. Classification runs in a goroutine per
// connection so a slow client cannot block Accept.
type listener struct {
	net.Listener
	ctx       context.Context
	tlsConfig *tls.Config
	mode      authz.Mode

	conns     chan net.Conn
	errs      chan error
	done      chan struct{}
	closeOnce sync.Once
}

func newListener(ctx context.Context, inner net.Listener, tlsConfig *tls.Config, mode authz.Mode) *listener {
	l := &listener{
		Listener:  inner,
		ctx:       ctx,
		tlsConfig: tlsConfig,
		mode:      mode,
		conns:     make(chan net.Conn),
		errs:      make(chan error),
		done:      make(chan struct{}),
	}
	go l.acceptLoop()
	return l
}

func (l *listener) acceptLoop() {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			select {
			case l.errs <- err:
			case <-l.done:
				return
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go l.handle(c)
	}
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case err := <-l.errs:
		return nil, err
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *listener) Close() error {
	l.closeOnce.Do(func() { close(l.done) })
	return l.Listener.Close()
}

func (l *listener) handle(c net.Conn) {
	conn, err := l.classify(c)
	if err != nil {
		_ = c.Close()
		return
	}

	select {
	case l.conns <- conn:
	case <-l.done:
		_ = conn.Close()
	}
}

// classify returns the connection to serve, or an error if it must be closed.
func (l *listener) classify(c net.Conn) (net.Conn, error) {
	_ = c.SetDeadline(time.Now().Add(handshakeTimeout))
	br := bufio.NewReader(c)
	first, err := br.Peek(1)
	if err != nil {
		return nil, err
	}
	peeked := &peekedConn{Conn: c, r: br}

	if first[0] != tlsHandshakeRecord {
		logPeerFailure(l.ctx, l.mode, c.RemoteAddr(), peerError("no client certificate: plain HTTP connection", nil))
		if l.mode == authz.ModeEnforce {
			_, _ = io.WriteString(c, "HTTP/1.0 400 Bad Request\r\n\r\nClient sent an HTTP request to an HTTPS server.\n")
			return nil, errors.New("plain HTTP rejected")
		}
		_ = c.SetDeadline(time.Time{})
		return peeked, nil
	}

	tlsConn := tls.Server(peeked, l.tlsConfig)
	if err := tlsConn.HandshakeContext(l.ctx); err != nil {
		logPeerFailure(l.ctx, l.mode, c.RemoteAddr(), err)
		return nil, err
	}
	_ = c.SetDeadline(time.Time{})
	return tlsConn, nil
}

// peekedConn replays bytes buffered while classifying the connection.
type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}
