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
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestInternalIdentityClientBounds(t *testing.T) {
	t.Parallel()
	options := authz.Options{Mode: authz.ModeEnforce, MTLSProvider: "external", Component: "controller", Namespace: "radius-system"}
	for _, endpoint := range []string{"https://ucp.radius-system.svc:9444", "http://attacker.example:9444",
		"http://ucp.radius-system.svc:8080", "http://user@ucp.radius-system.svc:9444",
		"http://ucp.radius-system.svc:9444?extra=true", "http://ucp.radius-system.svc:9444#fragment"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := WithInternalIdentityClient(http.NotFoundHandler(), options, &http.Client{}, endpoint)
			require.Error(t, err)
		})
	}
	_, err := WithInternalIdentityClient(http.NotFoundHandler(), options, nil, "http://ucp.radius-system.svc:9444")
	require.Error(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client := &internalIdentityClient{client: server.Client(), origin: server.URL}
	start := time.Now()
	_, err = client.observe(t.Context(), options, uuid.NewString(), time.Now)
	require.Error(t, err)
	require.GreaterOrEqual(t, time.Since(start), 5*time.Second)
	require.Less(t, time.Since(start), 8*time.Second, "the body read must share the total request deadline")
}

func TestInternalIdentityControllerProbe(t *testing.T) {
	t.Setenv("RADIUS_INTERNAL_AUTH_POD_UID", uuid.NewString())
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			receiver, key, now := tokenFixture(t)
			now.SetTime(time.Now().UTC())
			require.NoError(t, receiver.keys.update(t.Context(), false))
			receiver.receiver = "ucp"
			options := authz.Options{Mode: mode, MTLSProvider: "external", Component: "controller",
				Namespace: receiver.namespace, TokenDirectory: t.TempDir()}
			var discovery atomic.Int32
			api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				discovery.Add(1)
				switch r.URL.Path {
				case "/.well-known/openid-configuration":
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"issuer": "https://cluster.example", "jwks_uri": "https://ignored.example/keys",
					}))
				case "/openid/v1/jwks":
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"keys": []any{publicJWK(t, key)}}))
				default:
					t.Errorf("unexpected discovery or online token review: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer api.Close()
			ucpOptions := options
			ucpOptions.Component = "ucp"
			upstream := httptest.NewServer(receiver.authorizeToken(mode, []string{"controller"},
				internalIdentityHandler(http.NotFoundHandler(), ucpOptions, uuid.NewString(), now.Now)))
			defer upstream.Close()
			outgoing := validTokenClaims(now.Now())
			outgoing.Subject = "system:serviceaccount:radius-system:controller"
			outgoing.Kubernetes.ServiceAccount.Name = "controller"
			outgoing.Audience = jwt.ClaimStrings{"ucp.radius-system.svc"}
			require.NoError(t, os.WriteFile(filepath.Join(options.TokenDirectory, "ucp"),
				[]byte(signedToken(t, key, outgoing, "key-1", jwt.SigningMethodES256)), 0600))
			transport, err := NewClientTransport(options)
			require.NoError(t, err)
			base := transport.(*tokenTransport).base.(*http.Transport)
			base.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
			}
			defer base.CloseIdleConnections()
			config := &rest.Config{Host: api.URL, Transport: api.Client().Transport}
			handler, stop, err := NewExternalIdentityProbe(t.Context(), options, &http.Client{Transport: transport},
				"http://ucp.radius-system.svc:9444/apis/api.ucp.dev/v1alpha3", config)
			require.NoError(t, err)
			defer stop()
			server := httptest.NewServer(handler)
			defer server.Close()
			caller := validTokenClaims(now.Now())
			caller.Audience = jwt.ClaimStrings{"controller.radius-system.svc"}
			valid := signedToken(t, key, caller, "key-1", jwt.SigningMethodES256)
			for _, credential := range []string{"", "forged", valid} {
				request, err := http.NewRequest(http.MethodGet, server.URL+InternalIdentityPath+"?challenge="+uuid.NewString(), nil)
				require.NoError(t, err)
				request.Header.Set("Authorization", "Bearer "+credential)
				response, err := server.Client().Do(request)
				require.NoError(t, err)
				if credential == valid {
					require.Equal(t, http.StatusOK, response.StatusCode)
					var evidence struct {
						InternalIdentityReceipt
						Upstream InternalIdentityReceipt `json:"upstream"`
					}
					require.NoError(t, json.NewDecoder(response.Body).Decode(&evidence))
					require.Equal(t, "controller", evidence.Component)
					require.Equal(t, outgoing.Subject, evidence.Upstream.Caller)
					require.Equal(t, evidence.Challenge, evidence.Upstream.Challenge)
				} else {
					require.Equal(t, http.StatusUnauthorized, response.StatusCode)
				}
				require.NoError(t, response.Body.Close())
			}
			options.Mode = authz.ModeOff
			_, _, err = NewExternalIdentityProbe(t.Context(), options, nil, "", nil)
			require.Error(t, err)
			require.EqualValues(t, 2, discovery.Load(), "Off must not discover keys; requests must never use TokenReview")
			stop()
		})
	}
}

func TestInternalIdentityUsesRotatingProductionClient(t *testing.T) {
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			inbound, key, now := tokenFixture(t)
			upstreamVerifier := &tokenVerifier{keys: inbound.keys, namespace: inbound.namespace, receiver: "ucp"}
			ucpOptions := authz.Options{Mode: mode, MTLSProvider: "external", Namespace: inbound.namespace, Component: "ucp"}
			ucpPod := uuid.NewString()
			var calls atomic.Int32
			upstreamHandler := upstreamVerifier.authorizeToken(mode, []string{"applications-rp"},
				internalIdentityHandler(http.NotFoundHandler(), ucpOptions, ucpPod, now.Now))
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				upstreamHandler.ServeHTTP(w, r)
			}))
			defer upstream.Close()
			options := authz.Options{Mode: mode, MTLSProvider: "external", Component: "applications-rp",
				Namespace: inbound.namespace, TokenDirectory: t.TempDir()}
			transport, err := NewClientTransport(options)
			require.NoError(t, err)
			actualTransport := transport.(*tokenTransport).base.(*http.Transport)
			actualTransport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
			}
			t.Cleanup(actualTransport.CloseIdleConnections)
			client := &http.Client{Transport: transport}
			wrapped, err := WithInternalIdentityClient(http.NotFoundHandler(), options, client,
				"http://ucp."+inbound.namespace+".svc:9444/apis/api.ucp.dev/v1alpha3")
			require.NoError(t, err)
			podUID := uuid.NewString()
			server := httptest.NewServer(inbound.authorizeToken(mode, []string{"ucp"},
				internalIdentityHandler(wrapped, options, podUID, now.Now)))
			defer server.Close()
			callerToken := signedToken(t, key, validTokenClaims(now.Now()), "key-1", jwt.SigningMethodES256)
			for _, step := range []string{"valid", "rotated", "invalid", "wrong audience", "missing", "repaired"} {
				t.Run(step, func(t *testing.T) {
					path := filepath.Join(options.TokenDirectory, "ucp")
					outgoing := validTokenClaims(now.Now())
					outgoing.Audience = jwt.ClaimStrings{"ucp." + options.Namespace + ".svc"}
					outgoing.Subject = "system:serviceaccount:" + options.Namespace + ":applications-rp"
					outgoing.Kubernetes.ServiceAccount.Name = "applications-rp"
					outgoing.ID = uuid.NewString()
					expected := http.StatusOK
					if step == "wrong audience" {
						outgoing.Audience = jwt.ClaimStrings{"bicep-de." + options.Namespace + ".svc"}
						expected = http.StatusServiceUnavailable
					}
					token := signedToken(t, key, outgoing, "key-1", jwt.SigningMethodES256)
					if step == "invalid" {
						token, expected = "invalid-projection", http.StatusServiceUnavailable
					}
					require.NoError(t, os.WriteFile(path, []byte(token), 0600))
					if step == "missing" {
						require.NoError(t, os.Remove(path))
						expected = http.StatusServiceUnavailable
					}
					challenge := uuid.NewString()
					request, err := http.NewRequest(http.MethodGet, server.URL+InternalIdentityPath+"?challenge="+challenge, nil)
					require.NoError(t, err)
					request.Header.Set("Authorization", "Bearer "+callerToken)
					response, err := server.Client().Do(request)
					require.NoError(t, err)
					body, err := io.ReadAll(response.Body)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
					require.Equal(t, expected, response.StatusCode, string(body))
					require.NotContains(t, string(body), token)
					if expected == http.StatusOK {
						var evidence struct {
							InternalIdentityReceipt
							Upstream InternalIdentityReceipt `json:"upstream"`
						}
						require.NoError(t, json.Unmarshal(body, &evidence))
						require.Equal(t, podUID, evidence.PodUID)
						require.Equal(t, challenge, evidence.Challenge)
						require.Equal(t, ucpPod, evidence.Upstream.PodUID)
						require.Equal(t, challenge, evidence.Upstream.Challenge)
						require.Equal(t, outgoing.Subject, evidence.Upstream.Caller)
						require.Equal(t, processBootID, evidence.Upstream.BootID)
						require.Equal(t, now.Now().UTC(), evidence.Upstream.ObservedAt)
					}
				})
				now.Step(time.Second)
			}
			require.EqualValues(t, 5, calls.Load(), "every observation reads the current projection and performs a new request")
		})
	}
}

func TestInternalIdentityUpstreamValidation(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	options := authz.Options{Mode: authz.ModeEnforce, MTLSProvider: "external", Component: "controller", Namespace: "radius-system"}
	challenge := uuid.NewString()
	valid := InternalIdentityReceipt{SchemaVersion: 1, Provider: "external", Mode: options.Mode, Component: "ucp",
		Namespace: options.Namespace, PodUID: uuid.NewString(), BootID: uuid.NewString(),
		Caller: "system:serviceaccount:radius-system:controller", Challenge: challenge, ObservedAt: now}
	for _, scenario := range []string{"valid", "duplicate", "unknown", "wrong case", "null", "trailing", "missing",
		"wrong provider", "wrong mode", "wrong component", "wrong namespace", "wrong caller", "wrong challenge",
		"invalid pod", "nil boot", "stale", "future", "non UTC", "oversized", "redirect", "unavailable", "cancelled body"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			receipt := valid
			switch scenario {
			case "wrong provider":
				receipt.Provider = "radius"
			case "wrong mode":
				receipt.Mode = authz.ModeDryRun
			case "wrong component":
				receipt.Component = "controller"
			case "wrong namespace":
				receipt.Namespace = "other"
			case "wrong caller":
				receipt.Caller = "system:serviceaccount:radius-system:ucp"
			case "wrong challenge":
				receipt.Challenge = uuid.NewString()
			case "invalid pod":
				receipt.PodUID = "invalid"
			case "nil boot":
				receipt.BootID = uuid.Nil.String()
			case "stale":
				receipt.ObservedAt = now.Add(-31 * time.Second)
			case "future":
				receipt.ObservedAt = now.Add(31 * time.Second)
			case "non UTC":
				receipt.ObservedAt = now.In(time.FixedZone("offset", 3600))
			}
			raw, err := json.Marshal(receipt)
			require.NoError(t, err)
			body := string(raw)
			switch scenario {
			case "duplicate":
				body = strings.Replace(body, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1)
			case "unknown":
				body = strings.Replace(body, `"schemaVersion":1`, `"schemaVersion":1,"unknown":true`, 1)
			case "wrong case":
				body = strings.Replace(body, `"podUID"`, `"podUid"`, 1)
			case "null":
				body = strings.Replace(body, `"schemaVersion":1`, `"schemaVersion":null`, 1)
			case "trailing":
				body += "{}"
			case "missing":
				body = strings.Replace(body, `"schemaVersion":1,`, "", 1)
			case "oversized":
				body += strings.Repeat(" ", 8193)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "redirect":
					w.Header().Set("Location", "https://must-not-follow.invalid")
					w.WriteHeader(http.StatusFound)
				case "unavailable":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "cancelled body":
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				default:
					_, err := io.WriteString(w, body)
					require.NoError(t, err)
				}
			}))
			defer server.Close()
			client := &internalIdentityClient{client: server.Client(), origin: server.URL}
			ctx := t.Context()
			if scenario == "cancelled body" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 30*time.Millisecond)
				defer cancel()
			}
			got, err := client.observe(ctx, options, challenge, func() time.Time { return now })
			if scenario == "valid" {
				require.NoError(t, err)
				require.Equal(t, valid, got)
			} else {
				require.Error(t, err)
				require.Empty(t, got.PodUID)
			}
		})
	}
}
func TestInternalIdentityReceiptHTTP(t *testing.T) {
	t.Parallel()
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		for _, scenario := range []string{"valid", "forged", "missing", "replay", "wrong caller", "stale", "expired",
			"missing pod", "invalid pod", "missing challenge", "duplicate challenge", "nil challenge",
			"uppercase challenge", "extra query", "malformed query", "post"} {
			t.Run(string(mode)+"/"+scenario, func(t *testing.T) {
				t.Parallel()
				v, key, now := tokenFixture(t)
				v.receiver = "ucp"
				claims := validTokenClaims(now.Now())
				claims.Audience = jwt.ClaimStrings{"ucp.radius-system.svc"}
				claims.Kubernetes.ServiceAccount.Name = "bicep-de"
				if scenario == "wrong caller" {
					claims.Kubernetes.ServiceAccount.Name = "controller"
				}
				claims.Subject = "system:serviceaccount:radius-system:" + claims.Kubernetes.ServiceAccount.Name
				if scenario == "replay" {
					claims.Audience = jwt.ClaimStrings{"applications-rp.radius-system.svc"}
				}
				token := signedToken(t, key, claims, "key-1", jwt.SigningMethodES256)
				expected := http.StatusOK
				switch scenario {
				case "forged":
					token, expected = "forged", http.StatusUnauthorized
				case "missing":
					token, expected = "", http.StatusUnauthorized
				case "replay":
					expected = http.StatusUnauthorized
				case "wrong caller":
					expected = http.StatusForbidden
				case "stale":
					now.Step(15 * time.Minute)
					expected = http.StatusServiceUnavailable
				case "expired":
					now.Step(10 * time.Minute)
					expected = http.StatusUnauthorized
				}
				podUID, challenge := uuid.NewString(), uuid.NewString()
				query, method := "challenge="+challenge, http.MethodGet
				switch scenario {
				case "missing pod":
					podUID, expected = "", http.StatusServiceUnavailable
				case "invalid pod":
					podUID, expected = "not-a-uuid", http.StatusServiceUnavailable
				case "missing challenge":
					query, expected = "", http.StatusBadRequest
				case "duplicate challenge":
					query, expected = query+"&"+query, http.StatusBadRequest
				case "nil challenge":
					query, expected = "challenge="+uuid.Nil.String(), http.StatusBadRequest
				case "uppercase challenge":
					query, expected = "challenge=ABCDEFAB-1234-1234-1234-ABCDEFABCDEF", http.StatusBadRequest
				case "extra query":
					query, expected = query+"&url=http://attacker.example", http.StatusBadRequest
				case "malformed query":
					query, expected = query+"&broken=%ZZ", http.StatusBadRequest
				case "post":
					method, expected = http.MethodPost, http.StatusMethodNotAllowed
				}
				options := authz.Options{Mode: mode, MTLSProvider: "external", Component: "ucp", Namespace: v.namespace}
				business := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Error("identity probes must not invoke business routes")
				})
				server := httptest.NewServer(v.authorizeToken(mode, []string{"bicep-de"},
					internalIdentityHandler(business, options, podUID, now.Now)))
				defer server.Close()
				for range 2 {
					request, err := http.NewRequest(method, server.URL+InternalIdentityPath+"?"+query, nil)
					require.NoError(t, err)
					if token != "" {
						request.Header.Set("Authorization", "Bearer "+token)
					}
					request.Header.Set("X-Remote-User", "system:serviceaccount:radius-system:bicep-de")
					request.Header.Set("X-Forwarded-Client-Cert", "forged")
					response, err := server.Client().Do(request)
					require.NoError(t, err)
					body, err := io.ReadAll(response.Body)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
					require.Equal(t, expected, response.StatusCode, string(body))
					require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
					require.NotContains(t, string(body), "Bearer")
					if len(token) > 10 {
						require.NotContains(t, string(body), token)
					}
					if expected == http.StatusOK {
						var receipt InternalIdentityReceipt
						require.NoError(t, json.Unmarshal(body, &receipt))
						require.Equal(t, InternalIdentityReceipt{
							SchemaVersion: 1, Provider: "external", Mode: mode, Component: "ucp", Namespace: v.namespace,
							PodUID: podUID, BootID: processBootID, Caller: claims.Subject,
							Challenge: challenge, ObservedAt: now.Now().UTC(),
						}, receipt)
						require.True(t, strings.HasSuffix(receipt.ObservedAt.Format(time.RFC3339), "Z"))
					}
					now.Step(time.Second)
				}
			})
		}
	}
}
