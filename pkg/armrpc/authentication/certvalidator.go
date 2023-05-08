/*
------------------------------------------------------------
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
------------------------------------------------------------
*/
package authentication

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
				log.V(ucplog.Debug).Info("X-SSL-Client-Thumbprint header is missing")
				handleErr(r.Context(), w, r)
				return
			}
			isValid := IsValidThumbprint(clientRequestThumbprint)
			if !isValid {
				log.V(ucplog.Debug).Info("Thumbprint validating failed")
				handleErr(r.Context(), w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func handleErr(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	logger := logr.FromContextOrDiscard(ctx)
	resp := rest.NewClientAuthenticationFailedARMResponse()
	err := resp.Apply(req.Context(), w, req)
	if err != nil {
		// Responds with an HTTP 500
		body := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInternal,
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
