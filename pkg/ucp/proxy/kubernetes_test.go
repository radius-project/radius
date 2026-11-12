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

package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
)

func TestARMProxy_ComponentIdentityDoesNotPreserveForwardedUserHeaders(t *testing.T) {
	t.Parallel()

	downstream, err := url.Parse("https://applications-rp.radius-system.svc")
	require.NoError(t, err)

	for _, component := range authz.Components {
		t.Run(component, func(t *testing.T) {
			t.Parallel()
			identity := authz.Identity{Component: component, Namespace: "radius-system"}
			request := httptest.NewRequest(http.MethodGet, "/planes/radius/local/resourceGroups/test", nil)
			request = request.WithContext(authz.WithIdentity(t.Context(), identity))
			request.Header.Set("X-Remote-User", "cluster-admin")
			request.Header.Add("X-Remote-Group", "system:masters")
			request.Header.Set("X-Remote-Extra-Scope", "all")
			request.Header.Set("X-Ms-Correlation-Request-Id", "correlation")

			var forwarded http.Header
			proxy := NewARMProxy(ReverseProxyOptions{RoundTripper: &transportFunc{
				Func: func(r *http.Request) (*http.Response, error) {
					forwarded = r.Header.Clone()
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: r}, nil
				},
			}}, downstream, nil)
			response := httptest.NewRecorder()
			proxy.ServeHTTP(response, request)

			require.Equal(t, http.StatusOK, response.Code)
			require.NotNil(t, forwarded)
			require.Empty(t, forwarded.Values("X-Remote-User"))
			require.Empty(t, forwarded.Values("X-Remote-Group"))
			require.Empty(t, forwarded.Values("X-Remote-Extra-Scope"))
			require.Equal(t, "correlation", forwarded.Get("X-Ms-Correlation-Request-Id"))
			got, ok := authz.IdentityFromContext(request.Context())
			require.True(t, ok)
			require.Equal(t, identity, got)
		})
	}
}
