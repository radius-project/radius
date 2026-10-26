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

// Package authzcheck holds UCP's internal component authorization checks: who the
// caller is (A6), execution records on proxied writes (B2-B4), the namespace mapping
// for the controller (C2), and the data-access endpoint (D1).
package authzcheck

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"strings"

	"github.com/go-logr/logr"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// LogFieldPrototypeExemption marks a request allowed by a prototype shortcut. It is
	// not a would-deny: it is logged in both dryRun and enforce.
	LogFieldPrototypeExemption = "authzPrototypeExemption"

	// LogFieldCallerKind and LogFieldCallerName describe the resolved caller.
	LogFieldCallerKind = "authzCallerKind"
	LogFieldCallerName = "authzCallerName"

	// ComponentDeploymentEngine is the deployment engine's component name.
	ComponentDeploymentEngine = "bicep-de"

	// ComponentController is the Kubernetes controller's component name.
	ComponentController = "controller"

	remoteHeaderPrefix = "x-remote-"
)

// CallerKind is how a caller was authenticated.
type CallerKind string

const (
	// CallerUser is a Kubernetes user forwarded by the verified aggregation proxy.
	CallerUser CallerKind = "user"

	// CallerComponent is a Radius component with a verified mTLS certificate, or the
	// deployment engine under the prototype exemption.
	CallerComponent CallerKind = "component"

	// CallerAnonymous is a caller that could not be authenticated. It is only seen in
	// dryRun mode, or for discovery requests.
	CallerAnonymous CallerKind = "anonymous"
)

// Caller is the authenticated caller of a UCP request.
type Caller struct {
	Kind CallerKind
	Name string
	// Groups are populated only after the aggregation proxy certificate is verified.
	Groups        []string
	VerifiedProxy bool
}

func (c Caller) String() string {
	return string(c.Kind) + ":" + c.Name
}

type callerKey struct{}
type internalKey struct{}
type remoteAddrKey struct{}

// WithCaller returns a context that carries c.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFromContext returns the caller set by the middleware.
func CallerFromContext(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)
	return c, ok
}

// MarkInternal marks requests served on UCP's internal (mTLS) listener.
func MarkInternal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), internalKey{}, true)))
	})
}

func isInternal(ctx context.Context) bool {
	v, _ := ctx.Value(internalKey{}).(bool)
	return v
}

// CaptureRemoteAddr saves the remote address before later middleware clears it.
func CaptureRemoteAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), remoteAddrKey{}, r.RemoteAddr)))
	})
}

func remoteIP(ctx context.Context) string {
	addr, _ := ctx.Value(remoteAddrKey{}).(string)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// FrontProxyVerifier verifies the client certificate of the Kubernetes aggregation proxy.
type FrontProxyVerifier interface {
	Verify(ctx context.Context, certs []*x509.Certificate) error
}

// EngineResolver reports whether an IP address belongs to a deployment engine pod.
type EngineResolver interface {
	IsEngine(ctx context.Context, ip string) bool
}

// Authenticator resolves the caller of every UCP request.
type Authenticator struct {
	Mode       authz.Mode
	FrontProxy FrontProxyVerifier
	Engine     EngineResolver
}

// Middleware resolves the caller and rejects requests that cannot be authenticated:
//
//   - On the internal listener the caller is the component named by its mTLS
//     certificate. X-Remote-* headers are rejected: a component cannot assert a user.
//   - On the aggregation listener, X-Remote-* headers are trusted only when the
//     connection presents a client certificate that chains to the cluster's
//     requestheader CA and has an allowed name (A6).
//   - PROTOTYPE EXEMPTION: the deployment engine cannot present a certificate yet (A7),
//     so a request on the aggregation listener with no certificate and no X-Remote-*
//     headers is treated as bicep-de when it comes from a bicep-de pod IP.
//   - Other unauthenticated requests are allowed only for API discovery.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	if a.Mode == authz.ModeOff || a.Mode == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		caller, code := a.resolve(ctx, r)
		logger := ucplog.FromContextOrDiscard(ctx).WithValues(LogFieldCallerKind, string(caller.Kind), LogFieldCallerName, caller.Name)
		ctx = logr.NewContext(ctx, logger)

		decision := authz.Decision{Allowed: code == "", Code: code, Action: r.Method, Target: r.URL.Path}
		if err := authz.Apply(ctx, a.Mode, decision); err != nil {
			authz.WriteDenied(ctx, w, r, err)
			return
		}

		// Remote user headers never travel past UCP.
		stripRemoteHeaders(r.Header)

		next.ServeHTTP(w, r.WithContext(WithCaller(ctx, caller)))
	})
}

func (a *Authenticator) resolve(ctx context.Context, r *http.Request) (Caller, string) {
	hasRemote := hasRemoteHeaders(r.Header)

	if isInternal(ctx) {
		id, ok := mtls.IdentityFromContext(ctx)
		if hasRemote {
			name := ""
			if ok {
				name = id.Component
			}
			return Caller{Kind: CallerComponent, Name: name}, v1.CodeInvalidAuthenticationInfo
		}
		if !ok {
			// dryRun accepts plain HTTP; the mTLS layer already logged it.
			return Caller{Kind: CallerAnonymous}, ""
		}
		return Caller{Kind: CallerComponent, Name: id.Component}, ""
	}

	var certs []*x509.Certificate
	if r.TLS != nil {
		certs = r.TLS.PeerCertificates
	}

	if len(certs) > 0 {
		if a.FrontProxy == nil {
			return Caller{Kind: CallerAnonymous}, v1.CodeAuthorizationUnavailable
		}
		if err := a.FrontProxy.Verify(ctx, certs); err != nil {
			ucplog.FromContextOrDiscard(ctx).Info("aggregation proxy certificate rejected", "reason", err.Error())
			return Caller{Kind: CallerAnonymous}, v1.CodeInvalidAuthenticationInfo
		}
		return Caller{Kind: CallerUser, Name: r.Header.Get("X-Remote-User"), Groups: append([]string(nil), r.Header.Values("X-Remote-Group")...), VerifiedProxy: true}, ""
	}

	if hasRemote {
		// A forged X-Remote-User without the aggregation proxy's certificate.
		return Caller{Kind: CallerUser, Name: r.Header.Get("X-Remote-User")}, v1.CodeInvalidAuthenticationInfo
	}

	if a.Engine != nil && a.Engine.IsEngine(ctx, remoteIP(ctx)) {
		ucplog.FromContextOrDiscard(ctx).V(ucplog.LevelDebug).Info("deployment engine call accepted without a certificate (prototype exemption until A7)",
			LogFieldPrototypeExemption, "engine-no-cert", "authzPeerAddress", remoteIP(ctx))
		return Caller{Kind: CallerComponent, Name: ComponentDeploymentEngine}, ""
	}

	if isDiscovery(r.URL.Path) {
		return Caller{Kind: CallerAnonymous}, ""
	}

	return Caller{Kind: CallerAnonymous}, v1.CodeInvalidAuthenticationInfo
}

func hasRemoteHeaders(h http.Header) bool {
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), remoteHeaderPrefix) {
			return true
		}
	}
	return false
}

func stripRemoteHeaders(h http.Header) {
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), remoteHeaderPrefix) {
			h.Del(k)
		}
	}
}

// isDiscovery is true for API discovery paths, which carry no Radius resource.
func isDiscovery(path string) bool {
	p := strings.ToLower(path)
	return !strings.Contains(p, "/planes") && !strings.Contains(p, "/authz/")
}
