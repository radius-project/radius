// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicecontext

import (
	"fmt"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
)

// ARMRequestCtx is the middleware to inject ARMRequestContext to the http request.
func ARMRequestCtx(pathBase, location string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			rpcContext, err := v1.FromARMRequest(r, pathBase, location)
			if err != nil {
				resp := rest.NewBadRequestARMResponse(v1.ErrorResponse{
					Error: v1.ErrorDetails{
						Code:    v1.CodeInvalid,
						Message: fmt.Sprintf("unexpected error: %v", err),
					},
				})

				_ = resp.Apply(r.Context(), w, r)
				return
			}

			r = r.WithContext(v1.WithARMRequestContext(r.Context(), rpcContext))
			h.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
