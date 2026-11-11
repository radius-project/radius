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
	"k8s.io/client-go/rest"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/token"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// LogFieldReason is the log key for why authentication failed.
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

// Receiver describes the component a server runs as.
type Receiver struct {
	// Name is the receiving component, such as "ucp". Callers' tokens must have it as their
	// audience.
	Name string

	// AllowedCallers are the components that may call this receiver. Empty accepts any
	// Radius component.
	AllowedCallers []string
}

// ListenAndServe serves srv according to the authorization mode in options.
//
// In off mode it calls srv.ListenAndServe, so behavior is unchanged.
//
// Otherwise the server presents the certificate in options.CertificateDirectory, never
// requests a client certificate, and authenticates each request by its ServiceAccount
// token, whose audience must be receiver.Name.
//
// In dryRun mode the listener accepts both plain HTTP and TLS on the same port. Plain HTTP,
// missing or invalid tokens, and callers that are not in receiver.AllowedCallers are logged
// as would-deny, and every request is still served.
//
// In enforce mode the listener only accepts TLS. Plain HTTP is closed and logged as denied.
// A missing or invalid token gets InvalidAuthenticationInfo, a token that cannot be checked
// because the signing keys are stale gets AuthorizationUnavailable, and a caller that is not
// in receiver.AllowedCallers gets AuthorizationFailed.
//
// ListenAndServe changes srv's Handler when the mode is not off.
func ListenAndServe(ctx context.Context, srv *http.Server, options authz.Options, receiver Receiver) error {
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
	return Serve(ctx, srv, ln, options, receiver)
}

// Serve is like ListenAndServe but serves on an existing listener. It closes ln when
// it returns. In off mode it calls srv.Serve(ln).
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, options authz.Options, receiver Receiver) error {
	return serve(ctx, srv, ln, options, receiver, serveConfig{pollInterval: DefaultPollInterval})
}

// serveConfig holds dependencies that tests replace.
type serveConfig struct {
	// pollInterval is how often the certificate files are polled for changes.
	pollInterval time.Duration

	// keys is the source of token signing keys. Nil uses the hosting cluster's OIDC
	// discovery endpoint with the in-cluster config.
	keys token.KeySource

	// tokenNow is the verifier's clock. Nil uses time.Now.
	tokenNow func() time.Time
}

func serve(ctx context.Context, srv *http.Server, ln net.Listener, options authz.Options, receiver Receiver, config serveConfig) error {
	mode := options.EffectiveMode()
	if mode == authz.ModeOff {
		return srv.Serve(ln)
	}

	verifier, source, err := newServerAuth(options, receiver, config)
	if err != nil {
		_ = ln.Close()
		return err
	}

	// Load the signing keys before serving so requests right after startup are not answered
	// with AuthorizationUnavailable. Run keeps retrying if this fails.
	if err := verifier.Refresh(ctx); err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "Failed to load ServiceAccount token signing keys; retrying in the background")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go source.Start(runCtx, config.pollInterval)
	go verifier.Run(runCtx)

	s := &server{verifier: verifier, mode: mode, allowedCallers: receiver.AllowedCallers}

	handler := srv.Handler
	if handler == nil {
		handler = http.DefaultServeMux
	}
	srv.Handler = s.authorizeCaller(handler)

	return srv.Serve(newListener(ctx, ln, source.ServerConfig(), mode))
}

// newServerAuth loads the server certificate and creates the token verifier.
func newServerAuth(options authz.Options, receiver Receiver, config serveConfig) (*token.Verifier, *Source, error) {
	mode := options.EffectiveMode()
	if options.CertificateDirectory == "" || options.Namespace == "" {
		return nil, nil, fmt.Errorf("authorization mode %q requires certificateDirectory and namespace", mode)
	}
	if receiver.Name == "" {
		return nil, nil, errors.New("receiver name is required")
	}

	source, err := NewSourceWithCA(options.CertificateDirectory, options.EffectiveCADirectory())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	keys := config.keys
	if keys == nil {
		// Tokens are issued by the cluster hosting Radius, which may differ from the
		// deployment target cluster.
		restConfig, err := rest.InClusterConfig()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load in-cluster config for token verification: %w", err)
		}
		oidc, err := token.NewOIDCKeySource(restConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create token key source: %w", err)
		}
		keys = oidc
	}

	verifier, err := token.New(token.Config{
		Namespace: options.Namespace,
		Audience:  receiver.Name,
		Keys:      keys,
		Now:       config.tokenNow,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create token verifier: %w", err)
	}
	return verifier, source, nil
}

type server struct {
	verifier       *token.Verifier
	mode           authz.Mode
	allowedCallers []string
}

// authorizeCaller verifies the request's ServiceAccount token and checks that the caller is
// one of the allowed components. It removes the Authorization header before calling next, so
// the caller's token is never forwarded, for example when UCP proxies a request.
func (s *server) authorizeCaller(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		decision := authz.Decision{Allowed: true, Action: r.Method, Target: r.URL.Path}
		logger := ucplog.FromContextOrDiscard(ctx).WithValues(LogFieldPeerAddress, r.RemoteAddr)

		id, err := s.verifier.VerifyRequest(r)
		switch {
		case err != nil:
			decision.Allowed = false
			decision.Code = v1.CodeInvalidAuthenticationInfo
			var tokenErr *token.Error
			if errors.As(err, &tokenErr) {
				decision.Code = tokenErr.Code
			}
			logger = logger.WithValues(LogFieldReason, err.Error())
		case len(s.allowedCallers) > 0 && !slices.Contains(s.allowedCallers, id.Component):
			decision.Allowed = false
			decision.Code = authz.CodeAuthorizationFailed
			logger = logger.WithValues(LogFieldCaller, id.String())
		}
		ctx = logr.NewContext(ctx, logger)

		if err := authz.Apply(ctx, s.mode, decision); err != nil {
			writeDenied(ctx, w, r, err)
			return
		}

		r = r.Clone(r.Context())
		r.Header.Del("Authorization")
		if decision.Allowed {
			r = r.WithContext(authz.WithIdentity(r.Context(), id))
		}
		next.ServeHTTP(w, r)
	})
}

func writeDenied(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	var denied *authz.DeniedError
	if errors.As(err, &denied) {
		if resp, rerr := denied.Response(); rerr == nil {
			if aerr := resp.Apply(ctx, w, r); aerr == nil {
				return
			}
		}
	}
	http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
}

// logPlainHTTP logs a plain HTTP connection, which cannot carry a token safely, with the
// authorization log fields.
func logPlainHTTP(ctx context.Context, mode authz.Mode, remote string) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues(LogFieldReason, "plain HTTP connection", LogFieldPeerAddress, remote)
	_ = authz.Apply(logr.NewContext(ctx, logger), mode, authz.Decision{Code: v1.CodeInvalidAuthenticationInfo})
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
		logPlainHTTP(l.ctx, l.mode, c.RemoteAddr().String())
		if l.mode == authz.ModeEnforce {
			_, _ = io.WriteString(c, "HTTP/1.0 400 Bad Request\r\n\r\nClient sent an HTTP request to an HTTPS server.\n")
			return nil, errors.New("plain HTTP rejected")
		}
		_ = c.SetDeadline(time.Time{})
		return peeked, nil
	}

	tlsConn := tls.Server(peeked, l.tlsConfig)
	if err := tlsConn.HandshakeContext(l.ctx); err != nil {
		// Clients do not present certificates, so a failed handshake is not an authorization
		// decision.
		ucplog.FromContextOrDiscard(l.ctx).V(ucplog.LevelDebug).Info("TLS handshake failed", LogFieldPeerAddress, c.RemoteAddr().String(), "error", err.Error())
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
