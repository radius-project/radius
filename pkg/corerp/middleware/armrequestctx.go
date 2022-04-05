// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/servicecontext"
)

// ARMRequestCtx is the middleware to inject ARMRequestContext to the http request.
func ARMRequestCtx(pathBase string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			rpcContext, err := servicecontext.FromARMRequest(r, pathBase)
			if err != nil {
				h.ServeHTTP(w, r)
			}
			r = r.WithContext(servicecontext.WithARMRequestContext(r.Context(), rpcContext))
			h.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
