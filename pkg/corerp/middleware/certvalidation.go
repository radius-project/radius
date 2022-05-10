// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	armAuthenticator "github.com/project-radius/radius/pkg/corerp/authentication"
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
func ClientCertValidator(armCertMgr *armAuthenticator.ArmCertManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//skip cert validation for health and version endpoint
			log := radlogger.GetLogger(r.Context())
			if r.URL.Path == "/version" || r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			clientRequestThumbprint := r.Header.Get(IngressCertThumbprintHeader)
			if clientRequestThumbprint == "" {
				log.V(radlogger.Error).Info("X-SSL-Client-Thumbprint header is missing")
				handleErr(r.Context(), w, r)
				return
			}
			isValid, err := armCertMgr.IsValidThumbprint(clientRequestThumbprint)
			if err != nil {
				msg := fmt.Sprintf("Error validating the thumbprint %v", err)
				log.V(radlogger.Error).Info(msg)
				handleErr(r.Context(), w, r)
				return
			} else if !isValid {
				log.V(radlogger.Error).Info("Thumbprint validating failed")
				handleErr(r.Context(), w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Responds with an HTTP 500
func internalServerError(ctx context.Context, w http.ResponseWriter, req *http.Request, err error) {
	logger := radlogger.GetLogger(ctx)
	logger.Error(err, "unhandled error")

	// Try to use the ARM format to send back the error info
	body := armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Message: err.Error(),
		},
	}

	response := rest.NewInternalServerErrorARMResponse(body)
	err = response.Apply(ctx, w, req)
	if err != nil {
		// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
	}
}

func handleErr(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	resp := rest.NewClientAuthenticationFailedARMResponse()
	err := resp.Apply(req.Context(), w, req)
	if err != nil {
		internalServerError(req.Context(), w, req, err)
	}
}
