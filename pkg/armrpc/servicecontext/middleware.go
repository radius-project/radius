// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicecontext

import (
	"fmt"
	"net/http"

	"github.com/project-radius/radius/pkg/rp/armerrors"
	"github.com/project-radius/radius/pkg/rp/rest"
)

// ARMRequestCtx is the middleware to inject ARMRequestContext to the http request.
func ARMRequestCtx(pathBase string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			rpcContext, err := FromARMRequest(r, pathBase)
			if err != nil {
				resp := rest.NewBadRequestARMResponse(armerrors.ErrorResponse{
					Error: armerrors.ErrorDetails{
						Code:    armerrors.Invalid,
						Message: fmt.Sprintf("unexpected error: %v", err),
					},
				})

				_ = resp.Apply(r.Context(), w, r)
				return
			}

			r = r.WithContext(WithARMRequestContext(r.Context(), rpcContext))
			h.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
