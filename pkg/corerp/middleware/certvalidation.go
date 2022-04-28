// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	armAuthenticator "github.com/project-radius/radius/pkg/corerp/authentication"
	"github.com/project-radius/radius/pkg/radlogger"
)

const (
	IngressCertThumbprintHeader = "X-SSL-Client-Thumbprint"
	ArmCertificateRefreshRate   = 1 * time.Hour
)

// ValidateCerticate validates the thumbprint received in the request header with
// the active thumbprints fetched from ARM Metadata endpoint
func ValidateCerticate(armCertMgr *armAuthenticator.ArmCertManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//skip cert validation for health and version endpoint
			log := radlogger.GetLogger(r.Context())
			if strings.Contains(r.URL.Path, "healthz") || strings.Contains(r.URL.Path, "version") {
				next.ServeHTTP(w, r)
				return
			}
			clientRequestThumbprint := r.Header.Get(http.CanonicalHeaderKey(IngressCertThumbprintHeader))
			if clientRequestThumbprint == "" {
				log.V(radlogger.Error).Info("X-SSL-Client-Thumbprint header is missing")
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}
			isValid, err := armCertMgr.IsValidThumbprint(clientRequestThumbprint)
			if err != nil || !isValid {
				msg := fmt.Sprintf("Error validating the thumbprint %v", err)
				log.V(radlogger.Error).Info(msg)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}
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
