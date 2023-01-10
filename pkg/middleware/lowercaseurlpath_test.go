// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/assert"
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
		r := mux.NewRouter()
		r.Path("/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}").Methods(http.MethodPost).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				str := r.URL.Path + "|" + r.Header.Get(v1.RefererHeader)
				_, _ = w.Write([]byte(str))
			})

		handler := LowercaseURLPath(r)

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, testHostname+tt.armid, nil)
		if tt.refererHeader != "" {
			req.Header.Add(v1.RefererHeader, tt.refererHeader)
		}

		handler.ServeHTTP(w, req)

		parsed := strings.Split(w.Body.String(), "|")

		assert.Equal(t, tt.expected, parsed[0])
		assert.Equal(t, tt.armid, parsed[1][len(testHostname):])
	}
}
