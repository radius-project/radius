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

package authzcheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
)

type fakeFrontProxy struct{ err error }

func (f fakeFrontProxy) Verify(context.Context, []*x509.Certificate) error { return f.err }

type fakeEngine struct{ ip string }

func (f fakeEngine) IsEngine(_ context.Context, ip string) bool { return ip == f.ip }

func TestAuthenticator(t *testing.T) {
	t.Parallel()

	const resourcePath = "/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/c"

	cases := []struct {
		name       string
		mode       authz.Mode
		internal   bool
		identity   string
		cert       bool
		proxyErr   error
		remoteUser string
		remoteAddr string
		path       string
		wantStatus int
		wantCaller Caller
	}{
		{name: "verified proxy user", mode: authz.ModeEnforce, cert: true, remoteUser: "alice", wantStatus: 200, wantCaller: Caller{Kind: CallerUser, Name: "alice", VerifiedProxy: true}},
		{name: "forged remote user", mode: authz.ModeEnforce, remoteUser: "admin", wantStatus: 401},
		{name: "forged remote user dryRun", mode: authz.ModeDryRun, remoteUser: "admin", wantStatus: 200, wantCaller: Caller{Kind: CallerUser, Name: "admin"}},
		{name: "bad proxy cert", mode: authz.ModeEnforce, cert: true, proxyErr: errors.New("bad"), remoteUser: "alice", wantStatus: 401},
		{name: "bad proxy cert dryRun", mode: authz.ModeDryRun, cert: true, proxyErr: errors.New("bad"), remoteUser: "alice", wantStatus: 200, wantCaller: Caller{Kind: CallerAnonymous}},
		{name: "internal component asserting user dryRun", mode: authz.ModeDryRun, internal: true, identity: "dynamic-rp", remoteUser: "admin", wantStatus: 200, wantCaller: Caller{Kind: CallerComponent, Name: "dynamic-rp"}},
		{name: "engine exemption", mode: authz.ModeEnforce, remoteAddr: "10.0.0.5:1234", wantStatus: 200, wantCaller: Caller{Kind: CallerComponent, Name: ComponentDeploymentEngine}},
		{name: "anonymous resource", mode: authz.ModeEnforce, remoteAddr: "10.0.0.9:1234", wantStatus: 401},
		{name: "anonymous discovery", mode: authz.ModeEnforce, path: "/apis/api.ucp.dev/v1alpha3", wantStatus: 200, wantCaller: Caller{Kind: CallerAnonymous}},
		{name: "internal component", mode: authz.ModeEnforce, internal: true, identity: "dynamic-rp", wantStatus: 200, wantCaller: Caller{Kind: CallerComponent, Name: "dynamic-rp"}},
		{name: "internal component asserting user", mode: authz.ModeEnforce, internal: true, identity: "dynamic-rp", remoteUser: "admin", wantStatus: 401},
		{name: "off", mode: authz.ModeOff, remoteUser: "admin", wantStatus: 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got Caller
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, _ = CallerFromContext(r.Context())
				w.WriteHeader(200)
			})
			a := &Authenticator{Mode: tc.mode, FrontProxy: fakeFrontProxy{err: tc.proxyErr}, Engine: fakeEngine{ip: "10.0.0.5"}}
			var h http.Handler = a.Middleware(next)
			if tc.internal {
				h = MarkInternal(h)
			}
			h = CaptureRemoteAddr(h)

			path := tc.path
			if path == "" {
				path = resourcePath
			}
			req := httptest.NewRequest(http.MethodPut, path, nil)
			if tc.remoteAddr != "" {
				req.RemoteAddr = tc.remoteAddr
			}
			if tc.remoteUser != "" {
				req.Header.Set("X-Remote-User", tc.remoteUser)
			}
			if tc.cert {
				req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}}
			}
			if tc.identity != "" {
				req = req.WithContext(mtls.WithIdentity(req.Context(), mtls.Identity{Component: tc.identity, Namespace: "radius-system"}))
			}

			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			if tc.wantStatus == 200 && tc.mode != authz.ModeOff {
				require.Equal(t, tc.wantCaller, got)
			}
		})
	}
}
