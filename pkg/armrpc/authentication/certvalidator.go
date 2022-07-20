// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package authentication

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

const (
	IngressCertThumbprintHeader = "X-SSL-Client-Thumbprint"
	ArmCertificateRefreshRate   = 1 * time.Hour
)

// ClientCertValidator validates the thumbprint received in the request header with
// the active thumbprints fetched from ARM Metadata endpoint
func ClientCertValidator(armCertMgr *ArmCertManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//skip cert validation for health and version endpoint
			log := logr.FromContextOrDiscard(r.Context())
			if r.URL.Path == "/version" || r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			clientRequestThumbprint := r.Header.Get(IngressCertThumbprintHeader)
			if clientRequestThumbprint == "" {
				log.V(radlogger.Debug).Info("X-SSL-Client-Thumbprint header is missing")
				handleErr(r.Context(), w, r)
				return
			}
			isValid := IsValidThumbprint(clientRequestThumbprint)
			if !isValid {
				log.V(radlogger.Debug).Info("Thumbprint validating failed")
				handleErr(r.Context(), w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func handleErr(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	logger := radlogger.GetLogger(ctx)
	resp := rest.NewClientAuthenticationFailedARMResponse()
	err := resp.Apply(req.Context(), w, req)
	if err != nil {
		// Responds with an HTTP 500
		body := armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Code:    armerrors.InvalidAuthenticationInfo,
				Message: err.Error(),
			},
		}
		se := rest.NewInternalServerErrorARMResponse(body)
		err := se.Apply(req.Context(), w, req)
		if err != nil {
			logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
			// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}
