// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

func Recoverer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log := radlogger.GetLogger(r.Context())

				msg := fmt.Sprintf("recovering from panic %v: %s", err, debug.Stack())
				log.V(radlogger.Fatal).Info(msg)

				resp := rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
					Error: armerrors.ErrorDetails{
						Message: fmt.Sprintf("unexpected error: %v", err),
					},
				})

				_ = resp.Apply(r.Context(), w, r)
			}
		}()

		h.ServeHTTP(w, r)
	})
}
