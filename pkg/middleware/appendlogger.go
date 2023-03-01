// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// Append logger values to the context based on the Resource ID (if present).
func AppendLogValues(serviceName string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			id, err := resources.Parse(r.URL.Path)
			if err != nil {
				// This just means the request is for an ARM resource. Not an error.
				h.ServeHTTP(w, r)
				return
			}

			ctx := ucplog.WrapLogContext(r.Context(), ucplog.LogFieldResourceID, id.String())

			r = r.WithContext(ctx)
			h.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
