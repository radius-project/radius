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

package authentication

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const metadataEndpoint = "https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01"

func TestCertValidationUnauthorized(t *testing.T) {
	tests := []struct {
		armid    string
		expected string
	}{
		{
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourceGroups/proxy-rg/providers/Microsoft.Kubernetes/connectedClusters/mvm2a",
			"{\n  \"error\": {\n    \"code\": \"InvalidAuthenticationInfo\",\n    \"message\": \"Server failed to authenticate the request\"\n  }\n}",
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
		certificate, err := generateSignedCert()
		require.NoError(t, err)
		cert := Certificate{
			Certificate: certificate,
			NotAfter:    tm.Add(time.Minute * 15),
			NotBefore:   tm,
			Thumbprint:  "934367bf1c97033f877db0f15cb1b586957d313",
		}
		ctx := context.Background()
		log := logr.FromContextOrDiscard(ctx)
		armCertMgr := NewArmCertManager(metadataEndpoint, log)
		ArmCertStore.Store("934367bf1c97033f877db0f15cb1b586957d313", cert)
		r.Use(ClientCertValidator(armCertMgr))
		handler := middleware.LowercaseURLPath(r)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.armid, nil)
		req.Header.Set(IngressCertThumbprintHeader, "934367bf1c97033f877db0f15cb1b586957d312")
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
		tm := time.Now()
		certificate, err := generateSignedCert()
		require.NoError(t, err)
		cert := Certificate{
			Certificate: certificate,
			NotAfter:    tm.Add(time.Minute * 15),
			NotBefore:   tm,
			Thumbprint:  "934367bf1c97033f877db0f15cb1b586957d313",
		}
		ctx := context.Background()
		log := logr.FromContextOrDiscard(ctx)
		armCertMgr := NewArmCertManager(metadataEndpoint, log)
		ArmCertStore.Store("934367bf1c97033f877db0f15cb1b586957d313", cert)
		r.Use(ClientCertValidator(armCertMgr))
		handler := middleware.LowercaseURLPath(r)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.armid, nil)
		req.Header.Set(IngressCertThumbprintHeader, "934367bf1c97033f877db0f15cb1b586957d313")
		handler.ServeHTTP(w, req)
		parsed := w.Body.String()
		assert.Equal(t, tt.expected, parsed)
	}
}

func generateSignedCert() (string, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			Organization:  []string{"Radius Test Company"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Golden Gate Bridge"},
			PostalCode:    []string{"94016"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Minute * 15),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		fmt.Println("Failed to GenerateKey: ", err.Error())
		return "", err
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		fmt.Println("Failed to Generate certificate", err.Error())
		return "", err
	}

	return string(caBytes), nil
}
