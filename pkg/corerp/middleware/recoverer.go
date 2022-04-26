// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

const BufSize = 2048

func Recoverer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				buf := make([]byte, BufSize)
				size := runtime.Stack(buf, false)
				buf = buf[:size]

				log := radlogger.GetLogger(r.Context())
				msg := fmt.Sprintf("recovering from panic %v: %s", err, buf)
				log.V(radlogger.Fatal).Info(msg)

				resp := rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
					Error: armerrors.ErrorDetails{
						Message: msg,
					},
				})

				resp.Apply(r.Context(), w, r)
			}
		}()

		h.ServeHTTP(w, r)
	})
}
