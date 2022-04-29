// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	armAuthenticator "github.com/project-radius/radius/pkg/corerp/authentication"
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
		//create certiticate
		tm := time.Now()
		cert := armAuthenticator.Certificate{
			Certificate: "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",
			NotAfter:    tm.Add(time.Minute * 15),
			NotBefore:   tm,
			Thumbprint:  "16FD2BA9D0A534E7E1FB46955C29EF0558B81D4D",
		}
		armCertMgr := armAuthenticator.NewArmCertManager("https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01")
		(*sync.Map)(armCertMgr.CertStore).Store("16FD2BA9D0A534E7E1FB46955C29EF0558B81D4D", cert)
		r.Use(ClientCertValidator(armCertMgr))
		handler := LowercaseURLPath(r)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.armid, nil)
		req.Header.Set(IngressCertThumbprintHeader, "16FD2BA9D0A534E7E1FB46955C29EF0558B81D4Da")
		handler.ServeHTTP(w, req)
		parsed := w.Body.String()
		assert.Equal(t, tt.expected, parsed)
	}
}

func TestCertValidationAuthorized(t *testing.T) {
	tests := []struct {
		armid    string
		expected string
	}{
		{
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourceGroups/proxy-rg/providers/Microsoft.Kubernetes/connectedClusters/mvm2a",
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourcegroups/proxy-rg/providers/microsoft.kubernetes/connectedclusters/mvm2a",
		},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		r := mux.NewRouter()
		r.Path("/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}").Methods(http.MethodPost).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		//create certiticate
		tm := time.Now()
		cert := armAuthenticator.Certificate{
			Certificate: "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",
			NotAfter:    tm.Add(time.Minute * 15),
			NotBefore:   tm,
			Thumbprint:  "16FD2BA9D0A534E7E1FB46955C29EF0558B81D4D",
		}
		armCertMgr := armAuthenticator.NewArmCertManager("https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01")
		(*sync.Map)(armCertMgr.CertStore).Store("16FD2BA9D0A534E7E1FB46955C29EF0558B81D4D", cert)
		r.Use(ClientCertValidator(armCertMgr))
		handler := LowercaseURLPath(r)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.armid, nil)
		req.Header.Set(IngressCertThumbprintHeader, "16FD2BA9D0A534E7E1FB46955C29EF0558B81D4D")
		handler.ServeHTTP(w, req)
		parsed := w.Body.String()
		assert.Equal(t, tt.expected, parsed)
	}
}
