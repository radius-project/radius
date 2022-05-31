// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

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

	"github.com/gorilla/mux"
	armAuthenticator "github.com/project-radius/radius/pkg/corerp/authentication"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		cert := armAuthenticator.Certificate{
			Certificate: certificate,
			NotAfter:    tm.Add(time.Minute * 15),
			NotBefore:   tm,
			Thumbprint:  "934367bf1c97033f877db0f15cb1b586957d313",
		}
		ctx := context.Background()
		log := radlogger.GetLogger(ctx)
		armCertMgr := armAuthenticator.NewArmCertManager("https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01", log)
		armAuthenticator.ArmCertStore.Store("934367bf1c97033f877db0f15cb1b586957d313", cert)
		r.Use(ClientCertValidator(armCertMgr))
		handler := LowercaseURLPath(r)
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
		cert := armAuthenticator.Certificate{
			Certificate: certificate,
			NotAfter:    tm.Add(time.Minute * 15),
			NotBefore:   tm,
			Thumbprint:  "934367bf1c97033f877db0f15cb1b586957d313",
		}
		ctx := context.Background()
		log := radlogger.GetLogger(ctx)
		armCertMgr := armAuthenticator.NewArmCertManager("https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01", log)
		armAuthenticator.ArmCertStore.Store("934367bf1c97033f877db0f15cb1b586957d313", cert)
		r.Use(ClientCertValidator(armCertMgr))
		handler := LowercaseURLPath(r)
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
