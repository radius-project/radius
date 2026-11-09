// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/armrpc/rest"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// RequestHeaderOptions configures the Kubernetes authenticating proxy boundary.
// Trust is loaded at startup, independently of authorization enforcement modes.
type RequestHeaderOptions struct {
	// CAFile contains the cluster's requestheader-client-ca-file PEM bundle.
	// This must not be the general Kubernetes client CA.
	CAFile string `yaml:"caFile"`
	// AllowedNames contains requestheader-allowed-names (exact proxy certificate CNs).
	// An empty list permits any client name signed by the request-header CA,
	// matching Kubernetes authenticating proxy semantics.
	AllowedNames []string `yaml:"allowedNames"`
	// KubernetesIssuer is the installation's stable kubernetes:<trust-domain> issuer.
	KubernetesIssuer string `yaml:"kubernetesIssuer"`
}

// RequestHeaderAuthenticator maps verified Kubernetes proxy headers to Identity.
// Its trust snapshot is immutable; restart UCP to rotate the CA or allowed names.
type RequestHeaderAuthenticator struct {
	roots        *x509.CertPool
	allowedNames []string
	issuer       string
}

// NewRequestHeaderAuthenticator loads and validates the configured trust.
// With nil options it rejects all forwarded identity headers, while allowing
// headerless requests to continue without an identity.
func NewRequestHeaderAuthenticator(options *RequestHeaderOptions) (*RequestHeaderAuthenticator, error) {
	if options == nil {
		return &RequestHeaderAuthenticator{}, nil
	}
	if !strings.HasPrefix(options.KubernetesIssuer, "kubernetes:") ||
		strings.TrimSpace(strings.TrimPrefix(options.KubernetesIssuer, "kubernetes:")) == "" ||
		strings.ContainsAny(options.KubernetesIssuer, " \t\r\n") {
		return nil, fmt.Errorf("caller authentication requires a kubernetes:<trust-domain> issuer")
	}
	for _, name := range options.AllowedNames {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("caller authentication allowed names must not contain empty names")
		}
	}
	pem, err := os.ReadFile(options.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read request-header CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("request-header CA file contains no certificates")
	}
	return &RequestHeaderAuthenticator{
		roots: roots, allowedNames: slices.Clone(options.AllowedNames), issuer: options.KubernetesIssuer,
	}, nil
}

// TLSConfig requests client certificates for application-level proxy verification.
// It does not require a certificate from headerless callers or authenticate
// internal components, whose certificate trust belongs to a separate boundary.
func (a *RequestHeaderAuthenticator) TLSConfig() *tls.Config {
	if a.roots == nil {
		return nil
	}
	return &tls.Config{ClientAuth: tls.RequestClientCert, ClientCAs: a.roots.Clone()}
}

type identityContextKey struct{}

// IdentityFromContext returns only the identity established by request-header
// authentication. False means unidentified, not an anonymous authenticated user.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	return identity, ok
}

// Authenticate verifies the TLS peer against the dedicated request-header CA
// before reading a single asserted user or group. VerifiedChains from other TLS
// trust domains and forwarded certificate headers are deliberately not trusted.
func (a *RequestHeaderAuthenticator) Authenticate(r *http.Request) (Identity, error) {
	if hasRemoteHeaders(r.Trailer) {
		return Identity{}, fmt.Errorf("request-header identity must not be supplied in trailers")
	}
	if a.roots == nil {
		return Identity{}, fmt.Errorf("request-header authentication is not configured")
	}
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return Identity{}, fmt.Errorf("request-header authentication requires a TLS client certificate")
	}
	certs := r.TLS.PeerCertificates
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}
	if _, err := certs[0].Verify(x509.VerifyOptions{
		Roots: a.roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return Identity{}, fmt.Errorf("verify request-header client certificate: %w", err)
	}
	// x509.Verify also permits absent EKU and Any; proxy identity requires an
	// explicit clientAuth purpose on the leaf, not merely an unrestricted key.
	if !slices.Contains(certs[0].ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		return Identity{}, fmt.Errorf("request-header client certificate must explicitly permit client authentication")
	}
	if len(a.allowedNames) > 0 && !slices.Contains(a.allowedNames, certs[0].Subject.CommonName) {
		return Identity{}, fmt.Errorf("client certificate is not an allowed request-header proxy")
	}

	users := headerValues(r.Header, "X-Remote-User")
	if len(users) != 1 || strings.TrimSpace(users[0]) == "" {
		return Identity{}, fmt.Errorf("request-header authentication requires exactly one nonempty user")
	}
	typ := api.PrincipalTypeUser
	if strings.HasPrefix(users[0], "system:serviceaccount:") {
		parts := strings.Split(users[0], ":")
		if len(parts) != 4 || parts[2] == "" || parts[3] == "" {
			return Identity{}, fmt.Errorf("invalid Kubernetes service account identity")
		}
		typ = api.PrincipalTypeWorkload
	}
	issuer := a.issuer
	identity := Identity{Principal: api.Principal{Type: &typ, Issuer: &issuer, Subject: &users[0]}}
	if _, err := key(&identity.Principal); err != nil {
		return Identity{}, err
	}
	for _, subject := range headerValues(r.Header, "X-Remote-Group") {
		groupType := api.PrincipalTypeGroup
		group := api.Principal{Type: &groupType, Issuer: &issuer, Subject: &subject}
		if strings.TrimSpace(subject) == "" {
			return Identity{}, fmt.Errorf("empty Kubernetes group identity")
		}
		if _, err := key(&group); err != nil {
			return Identity{}, err
		}
		identity.Groups = append(identity.Groups, group)
	}
	return identity, nil
}

// Middleware rejects unverified forwarded headers in every enforcement mode.
// Headerless requests continue unidentified for compatibility; protected
// endpoints must require IdentityFromContext themselves. Remote headers are
// consumed here and never forwarded to downstream handlers. Announced remote
// trailers are always rejected before body consumption, even from trusted peers.
func (a *RequestHeaderAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasRemoteHeaders(r.Header) && !hasRemoteHeaders(r.Trailer) {
			next.ServeHTTP(w, r)
			return
		}
		identity, err := a.Authenticate(r)
		if err != nil {
			logger := ucplog.FromContextOrDiscard(r.Context())
			logger.V(ucplog.LevelDebug).Info("request-header authentication failed", "reason", err.Error())
			if err := rest.NewClientAuthenticationFailedARMResponse().Apply(r.Context(), w, r); err != nil {
				logger.Error(err, "failed to write authentication error response")
			}
			return
		}
		// Body consumption populates the original Trailer map; keep it shared.
		r = r.WithContext(context.WithValue(r.Context(), identityContextKey{}, identity))
		r.Header = r.Header.Clone()
		for name := range r.Header {
			if strings.HasPrefix(strings.ToLower(name), "x-remote-") {
				delete(r.Header, name)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func hasRemoteHeaders(headers http.Header) bool {
	for name := range headers {
		if strings.HasPrefix(strings.ToLower(name), "x-remote-") {
			return true
		}
	}
	return false
}

func headerValues(headers http.Header, name string) []string {
	var values []string
	for key, entries := range headers {
		if strings.EqualFold(key, name) {
			values = append(values, entries...)
		}
	}
	return values
}
