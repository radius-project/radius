// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/servicecontext"
)

// ARMRPCCtxInject injects ARMRPCContext to the http request.
func ARMRPCCtxInject(prefix string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			rpcContext, err := servicecontext.FromARMRPCRequest(r, prefix)
			if err != nil {
				h.ServeHTTP(w, r)
			}
			r = r.WithContext(servicecontext.WithARMRPCContext(r.Context(), rpcContext))
			h.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
