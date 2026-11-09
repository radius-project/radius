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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
)

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
