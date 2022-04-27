// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	armAuthenticator "github.com/project-radius/radius/pkg/corerp/authentication"
)

const (
	IngressCertThumbprintHeader = "X-SSL-Client-Thumbprint"
	ArmCertificateRefreshRate   = 1 * time.Hour
)

//The function validates the thumbprint received in the request header with
//the thumbprint fetched from ARM Metadata endpoint
func ValidateCerticate(armCertMgr *armAuthenticator.ArmCertManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//skip cert validation for health and version endpoint
			if strings.Contains(r.URL.Path, "healthz") || strings.Contains(r.URL.Path, "version") {
				next.ServeHTTP(w, r)
				return
			}
			clientRequestThumbprint := r.Header.Get(IngressCertThumbprintHeader)
			if clientRequestThumbprint == "" {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}
			armCertMgr.IsValidThumbprint(clientRequestThumbprint)
			next.ServeHTTP(w, r)
			return
		})
	}
}

//create a arm cert manager that
func NewArmCertManager(armMetaEndpoint string) (*armAuthenticator.ArmCertManager, error) {
	armCertManager := armAuthenticator.NewArmCertManager(armMetaEndpoint)
	_, err := armCertManager.Start(context.Background())
	if err != nil {
		return armCertManager, err
	}
	return armCertManager, nil
}
