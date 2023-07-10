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

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	testHostname = "http://localhost:1010"
)

func TestLowercaseURLPath(t *testing.T) {
	tests := []struct {
		armid         string
		refererHeader string
		expected      string
	}{
		{
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourceGroups/proxy-rg/providers/Microsoft.Kubernetes/connectedClusters/mvm2a",
			testHostname + "/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourceGroups/proxy-rg/providers/Microsoft.Kubernetes/connectedClusters/mvm2a",
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourcegroups/proxy-rg/providers/microsoft.kubernetes/connectedclusters/mvm2a",
		},
		{
			"/SubscriptionS/1F43AEF5-7868-4c56-8a7f-cb6822a75c0e/RESOURCEGroups/proxy-rg/providers/Microsoft.Kubernetes/connectedClusters/mvm2a",
			"",
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourcegroups/proxy-rg/providers/microsoft.kubernetes/connectedclusters/mvm2a",
		},
	}

	for _, tt := range tests {
		w := httptest.NewRecorder()
		r := chi.NewRouter()
		r.MethodFunc(
			http.MethodPost,
			"/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}",
			func(w http.ResponseWriter, r *http.Request) {
				str := r.URL.Path + "|" + r.Header.Get(v1.RefererHeader)
				_, _ = w.Write([]byte(str))
			})

		handler := LowercaseURLPath(r)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, testHostname+tt.armid, nil)
		require.NoError(t, err)
		if tt.refererHeader != "" {
			req.Header.Add(v1.RefererHeader, tt.refererHeader)
		}

		handler.ServeHTTP(w, req)

		parsed := strings.Split(w.Body.String(), "|")

		require.Equal(t, tt.expected, parsed[0])
		require.Equal(t, tt.armid, parsed[1][len(testHostname):])
	}
}
