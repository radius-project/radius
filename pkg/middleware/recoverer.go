// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

func Recoverer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log := ucplog.FromContextOrDiscard(r.Context())

				msg := fmt.Sprintf("recovering from panic %v: %s", err, debug.Stack())
				log.V(ucplog.Error).Info(msg)

				resp := rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
					Error: v1.ErrorDetails{
						Code:    v1.CodeInternal,
						Message: fmt.Sprintf("unexpected error: %v", err),
					},
				})

				_ = resp.Apply(r.Context(), w, r)
			}
		}()

		h.ServeHTTP(w, r)
	})
}
