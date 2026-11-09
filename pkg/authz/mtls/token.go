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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/rest"
	"k8s.io/utils/clock"
)

type componentTokenClaims struct {
	jwt.RegisteredClaims
	Kubernetes struct {
		Namespace      string `json:"namespace"`
		ServiceAccount struct {
			Name string `json:"name"`
			UID  string `json:"uid"`
		} `json:"serviceaccount"`
	} `json:"kubernetes.io"`
}

func (c *componentTokenClaims) UnmarshalJSON(data []byte) error {
	// Kubernetes issues integer NumericDates. Reject fractional or string dates
	// instead of allowing the JWT parser to round them to its time precision.
	var times struct {
		ExpiresAt *int64 `json:"exp"`
		IssuedAt  *int64 `json:"iat"`
		NotBefore *int64 `json:"nbf"`
	}
	if err := json.Unmarshal(data, &times); err != nil {
		return errors.New("invalid token time claims")
	}
	type claims componentTokenClaims
	return json.Unmarshal(data, (*claims)(c))
}

type tokenVerifier struct {
	keys      *tokenKeys
	namespace string
	receiver  string
}

func validComponent(name string) bool {
	return slices.Contains(Components, name)
}

func (v *tokenVerifier) verify(ctx context.Context, raw string) (Identity, string) {
	set, fresh := v.keys.snapshot()
	if !fresh {
		return Identity{}, v1.CodeAuthorizationUnavailable
	}
	if raw == "" || len(raw) > 16384 {
		return Identity{}, v1.CodeInvalidAuthenticationInfo
	}
	claims := &componentTokenClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" || len(kid) > 256 {
			return nil, errors.New("invalid key ID")
		}
		key, found := set.keys[kid]
		if !found {
			if err := v.keys.update(ctx, true); err != nil && ctx.Err() == nil {
				ucplog.FromContextOrDiscard(ctx).Error(err, "cannot refresh cluster signing keys")
			}
			current, fresh := v.keys.snapshot()
			if !fresh {
				return nil, errors.New("cluster signing keys unavailable")
			}
			key, found = current.keys[kid]
		}
		if !found || token.Method.Alg() != key.algorithm {
			return nil, errors.New("unsupported signing key")
		}
		return key.public, nil
	}, jwt.WithValidMethods([]string{"RS256", "ES256", "ES384"}),
		jwt.WithIssuer(set.issuer), jwt.WithAudience(v.receiver+"."+v.namespace+".svc"),
		jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(v.keys.clock.Now))
	// A slow refresh must not extend the last-good set beyond the hard boundary.
	if _, fresh := v.keys.snapshot(); !fresh {
		return Identity{}, v1.CodeAuthorizationUnavailable
	}
	if err != nil || claims.IssuedAt == nil || claims.NotBefore == nil ||
		claims.ExpiresAt == nil || !claims.ExpiresAt.After(claims.IssuedAt.Time) ||
		claims.NotBefore.After(claims.ExpiresAt.Time) || len(claims.Audience) != 1 {
		return Identity{}, v1.CodeInvalidAuthenticationInfo
	}
	sa := claims.Kubernetes.ServiceAccount
	if claims.Kubernetes.Namespace != v.namespace || !validComponent(sa.Name) || sa.UID == "" ||
		claims.Subject != "system:serviceaccount:"+v.namespace+":"+sa.Name {
		return Identity{}, v1.CodeInvalidAuthenticationInfo
	}
	return Identity{Component: sa.Name, Namespace: v.namespace}, ""
}

// authorizeToken keeps credential failures out of context, including in DryRun. The
// stale-key bound is an authentication availability limit and is always fail-closed.
func (v *tokenVerifier) authorizeToken(mode authz.Mode, allowed []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := ""
		values := r.Header.Values("Authorization")
		if len(values) == 1 {
			fields := strings.Fields(values[0])
			if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
				raw = fields[1]
			}
		}
		id, code := v.verify(r.Context(), raw)
		r = r.Clone(r.Context())
		// Internal callers cannot assert the Kubernetes aggregation proxy's identity.
		// Consume hop credentials so they cannot leak to unrelated downstreams.
		for header := range r.Header {
			lower := strings.ToLower(header)
			if lower == "authorization" || lower == "x-forwarded-client-cert" ||
				strings.HasPrefix(lower, "x-remote-") {
				delete(r.Header, header)
			}
		}
		// Mask any inherited identity: only this request's credential can set it.
		ctx := WithIdentity(r.Context(), Identity{})
		if code == "" {
			ctx = WithIdentity(ctx, id)
			if len(allowed) != 0 && !slices.Contains(allowed, id.Component) {
				code = v1.CodeAuthorizationFailed
			}
		}
		decision := authz.Decision{Allowed: code == "", Code: code, Action: r.Method, Target: r.URL.Path}
		effective := mode
		if code == v1.CodeAuthorizationUnavailable || r.URL.Path == InternalIdentityPath {
			effective = authz.ModeEnforce
		}
		if r.URL.Path == InternalIdentityPath {
			w.Header().Set("Cache-Control", "no-store")
		}
		if err := authz.Apply(ctx, effective, decision); err != nil {
			writeDenied(ctx, w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func serveExternal(ctx context.Context, srv *http.Server, ln net.Listener, options authz.Options, allowed []string) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		_ = ln.Close()
		return errors.New("external identity requires in-cluster signing key discovery")
	}
	return serveExternalWithConfig(ctx, srv, ln, options, allowed, config)
}

func serveExternalWithConfig(ctx context.Context, srv *http.Server, ln net.Listener, options authz.Options, allowed []string, config *rest.Config) error {
	defer ln.Close()
	handler, stop, err := externalTokenHandler(ctx, options, allowed, srv.Handler, config)
	if err != nil {
		return err
	}
	defer stop()
	srv.Handler = handler
	return srv.Serve(ln)
}

// NewExternalIdentityProbe protects a caller-only component's existing management
// endpoint and exercises the actual configured UCP client on each observation.
// The caller must invoke stop when its service terminates.
func NewExternalIdentityProbe(ctx context.Context, options authz.Options, client *http.Client, endpoint string, config *rest.Config) (handler http.Handler, stop func(), err error) {
	if options.EffectiveMTLSProvider() != "external" || options.EffectiveMode() == authz.ModeOff {
		return nil, nil, errors.New("internal identity probe requires active external identity")
	}
	next, err := WithInternalIdentityClient(http.NotFoundHandler(), options, client, endpoint)
	if err != nil {
		return nil, nil, err
	}
	return externalTokenHandler(ctx, options, []string{"ucp"}, next, config)
}

func externalTokenHandler(ctx context.Context, options authz.Options, allowed []string, handler http.Handler, config *rest.Config) (http.Handler, func(), error) {
	if _, err := authz.ParseMode(string(options.EffectiveMode())); err != nil {
		return nil, nil, err
	}
	if options.EffectiveMTLSProvider() != "external" || options.EffectiveMode() == authz.ModeOff {
		return nil, nil, errors.New("token verifier requires active external identity")
	}
	if !validComponent(options.Component) || len(validation.IsDNS1123Label(options.Namespace)) != 0 {
		return nil, nil, fmt.Errorf("external identity requires a known component and valid Radius namespace")
	}
	fetch, err := clusterTokenKeyFetcher(config)
	if err != nil {
		return nil, nil, err
	}
	keys := &tokenKeys{clock: clock.RealClock{}, fetch: fetch}
	sourceCtx, cancel := context.WithCancel(ctx)
	if err := keys.update(sourceCtx, false); err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "cluster signing keys unavailable; internal requests will fail closed")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		keys.run(sourceCtx)
	}()
	if handler == nil {
		handler = http.DefaultServeMux
	}
	verifier := &tokenVerifier{keys: keys, namespace: options.Namespace, receiver: options.Component}
	handler = internalIdentityHandler(handler, options, os.Getenv("RADIUS_INTERNAL_AUTH_POD_UID"), keys.clock.Now)
	return verifier.authorizeToken(options.EffectiveMode(), allowed, handler), func() { cancel(); <-done }, nil
}
