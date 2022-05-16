// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	"github.com/go-logr/logr"
)

// UseLogValues appends logging values to the context based on the request.
func UseLogValues(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		values := []interface{}{}
		logger := logr.FromContextOrDiscard(r.Context()).WithValues(values...)
		r = r.WithContext(logr.NewContext(r.Context(), logger))
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
