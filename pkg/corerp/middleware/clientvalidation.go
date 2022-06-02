// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/project-radius/radius/pkg/corerp/authentication"
	armAuthenticator "github.com/project-radius/radius/pkg/corerp/authentication"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

const (
	IngressCertThumbprintHeader = "X-SSL-Client-Thumbprint"
	ArmCertificateRefreshRate   = 1 * time.Hour
)

//ClientValidator validates the PoP token and if the auth header is not present then it falls back to the cert validation
func ClientValidator(identityOptions hostoptions.IdentityOptions, armCertMgr *armAuthenticator.ArmCertManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//skip cert validation for health and version endpoint
			log := radlogger.GetLogger(r.Context())
			if r.URL.Path == "/version" || r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			token, isAuthPresent, err := authentication.ValidateAuthHeader(r, log)
			if !isAuthPresent {
				err := clientCertValidator(r, armCertMgr)
				if err != nil {
					log.V(radlogger.Error).Info(err.Error())
					handleErr(r.Context(), w, r)
					return
				}
			} else {
				err = authentication.Validate(token, identityOptions, log)
				if err != nil {
					handleErr(r.Context(), w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientCertValidator validates the thumbprint received in the request header with
// the active thumbprints fetched from ARM Metadata endpoint
func clientCertValidator(r *http.Request, armCertMgr *armAuthenticator.ArmCertManager) error {
	//skip cert validation for health and version endpoint
	clientRequestThumbprint := r.Header.Get(IngressCertThumbprintHeader)
	if clientRequestThumbprint == "" {
		return errors.New("X-SSL-Client-Thumbprint header is missing")
	}
	isValid := authentication.IsValidThumbprint(clientRequestThumbprint)
	if !isValid {
		return errors.New("Thumbprint validating failed")
	}
	return nil
}

func handleErr(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	logger := radlogger.GetLogger(ctx)
	resp := rest.NewClientAuthenticationFailedARMResponse()
	err := resp.Apply(req.Context(), w, req)
	if err != nil {
		// Responds with an HTTP 500
		body := armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Message: err.Error(),
			},
		}
		se := rest.NewInternalServerErrorARMResponse(body)
		err := se.Apply(req.Context(), w, req)
		if err != nil {
			// There's no way to recover if we fail writing here, we likly partially wrote to the response stream.
			w.WriteHeader(http.StatusInternalServerError)
			logger.Error(err, fmt.Sprintf("error writing marshaled %T bytes to output", body))
		}
	}
}
