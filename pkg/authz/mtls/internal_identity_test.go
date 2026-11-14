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
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
)

func receiptServer(t *testing.T, ca testCA, options authz.Options, receiver string, callers []string, handler http.Handler) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if options.EffectiveTLSProvider() == authz.TLSProviderRadius {
		writeFiles(t, dir, ca.issue(t, certOptions{uris: spiffeURIs(receiver)}), ca.pem)
		options.CertificateDirectory = dir
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &http.Server{Handler: handler}
	if receiver == "ucp" {
		server.Handler = WithInternalIdentityHandler(handler, options, receiver)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, server, listener, options, Receiver{Name: receiver, AllowedCallers: callers, KeySource: &staticKeys{}},
			serveConfig{pollInterval: testPollInterval})
	}()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, server.Close())
		require.ErrorIs(t, <-done, http.ErrServerClosed)
	})
	return listener.Addr().String(), dir
}

func receiptClient(t *testing.T, options authz.Options, receiver, token, address, caDir string) *http.Client {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, receiver), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, receiver, TokenFile), []byte(token), 0600))
	options.TokenDirectory, options.CADirectory = dir, caDir
	options.CertificateDirectory = caDir
	transport, err := NewClientTransport(options, receiver)
	require.NoError(t, err)
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	switch tr := transport.(type) {
	case *clientTransport:
		tr.DialContext = dial
		t.Cleanup(tr.CloseIdleConnections)
	case *externalTransport:
		tr.DialContext = dial
		t.Cleanup(tr.CloseIdleConnections)
	default:
		t.Fatalf("unexpected common transport %T", transport)
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

func TestInternalIdentityCommonRequestPath(t *testing.T) {
	t.Setenv("RADIUS_POD_UID", uuid.NewString())
	for _, provider := range []authz.TLSProvider{authz.TLSProviderRadius, authz.TLSProviderExternal} {
		for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
			t.Run(string(provider)+"/"+string(mode), func(t *testing.T) {
				ca := newCA(t)
				options := authz.Options{Namespace: testNamespace, TLSProvider: provider, Mode: mode}
				scheme := "https"
				if provider == authz.TLSProviderExternal {
					scheme = "http"
				}
				ucpAddress, ucpCA := receiptServer(t, ca, options, "ucp", []string{"applications-rp"}, http.NotFoundHandler())
				upstream := receiptClient(t, options, "ucp", mintToken(t, "applications-rp", "ucp"), ucpAddress, ucpCA)
				handler, err := WithInternalIdentityClient(http.NotFoundHandler(), options, "applications-rp", upstream, scheme+"://ucp."+testNamespace+".svc:9444")
				require.NoError(t, err)
				rpAddress, rpCA := receiptServer(t, ca, options, "applications-rp", []string{"ucp"}, handler)
				client := receiptClient(t, options, "applications-rp", mintToken(t, "ucp", "applications-rp"), rpAddress, rpCA)
				challenge := uuid.NewString()
				endpoint := scheme + "://applications-rp." + testNamespace + ".svc:5443" + InternalIdentityPath + "?challenge=" + challenge
				response, err := client.Get(endpoint)
				require.NoError(t, err)
				defer response.Body.Close()
				require.Equal(t, http.StatusOK, response.StatusCode)
				require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
				var receipt struct {
					InternalIdentityReceipt
					Upstream InternalIdentityReceipt `json:"upstream"`
				}
				require.NoError(t, json.NewDecoder(response.Body).Decode(&receipt))
				require.Equal(t, string(provider), receipt.Provider)
				require.Equal(t, mode, receipt.Mode)
				require.Equal(t, "system:serviceaccount:"+testNamespace+":ucp", receipt.Caller)
				require.Equal(t, challenge, receipt.Challenge)
				require.Equal(t, string(provider), receipt.Upstream.Provider)
				require.Equal(t, mode, receipt.Upstream.Mode)
				require.Equal(t, "system:serviceaccount:"+testNamespace+":applications-rp", receipt.Upstream.Caller)
				require.Equal(t, challenge, receipt.Upstream.Challenge)
				require.True(t, validProofUUID(receipt.PodUID) && validProofUUID(receipt.BootID))

				for _, invalidCase := range []struct {
					token  string
					status int
				}{{"invalid", http.StatusUnauthorized}, {mintToken(t, "ucp", "ucp"), http.StatusUnauthorized},
					{mintToken(t, "controller", "applications-rp"), http.StatusForbidden}} {
					invalid := receiptClient(t, options, "applications-rp", invalidCase.token, rpAddress, rpCA)
					request, err := http.NewRequest(http.MethodGet, endpoint, nil)
					require.NoError(t, err)
					request.Header.Set("X-Forwarded-Client-Cert", "trusted")
					request.Header.Set("X-Remote-User", "system:serviceaccount:"+testNamespace+":ucp")
					response, err := invalid.Do(request)
					require.NoError(t, err)
					require.Equal(t, invalidCase.status, response.StatusCode, "proof must remain strict in DryRun")
					require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
					require.NoError(t, response.Body.Close())
				}
				response, err = client.Get(endpoint + "&challenge=" + challenge)
				require.NoError(t, err)
				require.Equal(t, http.StatusBadRequest, response.StatusCode)
				require.NoError(t, response.Body.Close())
			})
		}
	}
}

func TestInternalIdentityTimestampWirePrecision(t *testing.T) {
	for _, timestamp := range []string{"2026-11-13T21:15:22.123456700Z", "2026-11-13T21:15:22.1234567Z"} {
		body := `{"schemaVersion":1,"provider":"radius","mode":"enforce","component":"ucp","namespace":"radius-system","podUID":"` + uuid.NewString() + `","bootID":"` + uuid.NewString() + `","caller":"system:serviceaccount:radius-system:bicep-de","challenge":"` + uuid.NewString() + `","observedAt":"` + timestamp + `"}`
		receipt, err := DecodeInternalIdentityReceipt([]byte(body))
		require.NoError(t, err)
		encoded, err := marshalInternalIdentityReceipt(receipt)
		require.NoError(t, err)
		require.Contains(t, string(encoded), `"observedAt":"`+timestamp+`"`)
		_, err = DecodeInternalIdentityReceipt([]byte(strings.ReplaceAll(body, "Z\"", "+00:00\"")))
		require.Error(t, err)
		_, err = DecodeInternalIdentityReceipt([]byte(strings.Replace(body, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1)))
		require.Error(t, err)
	}
}

func TestInternalIdentityUnavailableWithoutUpstreamOrPod(t *testing.T) {
	options := authz.Options{Namespace: testNamespace, Mode: authz.ModeEnforce}
	for _, pod := range []string{"", uuid.NewString()} {
		handler := internalIdentityHandler(http.NotFoundHandler(), options, "applications-rp", pod, time.Now)
		request := httptest.NewRequest(http.MethodGet, InternalIdentityPath+"?challenge="+uuid.NewString(), nil)
		request = request.WithContext(authz.WithIdentity(request.Context(), authz.Identity{Namespace: testNamespace, Component: "ucp"}))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		_, err := io.ReadAll(recorder.Result().Body)
		require.NoError(t, err)
	}

}

func TestInternalIdentityUpstreamContract(t *testing.T) {
	now := time.Now().UTC()
	challenge := uuid.NewString()
	for _, tt := range []struct {
		name   string
		change func(*InternalIdentityReceipt)
		status int
		cache  string
		extra  string
		fail   bool
	}{
		{name: "valid", cache: "no-store"},
		{name: "provider mismatch", cache: "no-store", fail: true, change: func(r *InternalIdentityReceipt) { r.Provider = "external" }},
		{name: "mode mismatch", cache: "no-store", fail: true, change: func(r *InternalIdentityReceipt) { r.Mode = authz.ModeDryRun }},
		{name: "mapped caller not wire subject", cache: "no-store", fail: true, change: func(r *InternalIdentityReceipt) { r.Caller = "applications-rp." + testNamespace + ".svc" }},
		{name: "nonce mismatch", cache: "no-store", fail: true, change: func(r *InternalIdentityReceipt) { r.Challenge = uuid.NewString() }},
		{name: "expired", cache: "no-store", fail: true, change: func(r *InternalIdentityReceipt) { r.ObservedAt = now.Add(-31 * time.Second) }},
		{name: "future", cache: "no-store", fail: true, change: func(r *InternalIdentityReceipt) { r.ObservedAt = now.Add(31 * time.Second) }},
		{name: "missing process", cache: "no-store", fail: true, change: func(r *InternalIdentityReceipt) { r.BootID = "" }},
		{name: "cache permitted", fail: true},
		{name: "redirect", cache: "no-store", status: http.StatusTemporaryRedirect, fail: true},
		{name: "oversized", cache: "no-store", extra: strings.Repeat(" ", 8193), fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			receipt := InternalIdentityReceipt{SchemaVersion: 1, Provider: "radius", Mode: authz.ModeEnforce,
				Component: "ucp", Namespace: testNamespace, PodUID: uuid.NewString(), BootID: uuid.NewString(),
				Caller: "system:serviceaccount:" + testNamespace + ":applications-rp", Challenge: challenge, ObservedAt: now}
			if tt.change != nil {
				tt.change(&receipt)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, challenge, r.URL.Query().Get("challenge"))
				require.Equal(t, "no-store", r.Header.Get("Cache-Control"))
				w.Header().Set("Cache-Control", tt.cache)
				if tt.status != 0 {
					w.Header().Set("Location", "/untrusted")
					w.WriteHeader(tt.status)
				}
				require.NoError(t, json.NewEncoder(w).Encode(receipt))
				_, err := io.WriteString(w, tt.extra)
				require.NoError(t, err)
			}))
			defer server.Close()
			client := internalIdentityClient{client: server.Client(), origin: server.URL}
			_, err := client.observe(t.Context(), authz.Options{Mode: authz.ModeEnforce, Namespace: testNamespace},
				"applications-rp", challenge, func() time.Time { return now })
			if tt.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
