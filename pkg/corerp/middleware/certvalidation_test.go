// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestCertValidationUnauthorized(t *testing.T) {
	tests := []struct {
		armid    string
		expected string
	}{
		{
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourceGroups/proxy-rg/providers/Microsoft.Kubernetes/connectedClusters/mvm2a",
			"Unauthorized\n",
		},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		r := mux.NewRouter()
		r.Path("/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}").Methods(http.MethodPost).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		armCertMgr, err := NewArmCertManager("https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01")
		if err != nil || armCertMgr == nil {
			fmt.Println("error getting arm certs")
		}
		r.Use(ValidateCerticate(armCertMgr))
		handler := LowercaseURLPath(r)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.armid, nil)
		handler.ServeHTTP(w, req)
		parsed := w.Body.String()
		assert.Equal(t, tt.expected, parsed)
	}
}
