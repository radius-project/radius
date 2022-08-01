// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// UseLogValues appends logging values to the context based on the request.
func UseLogValues(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		values := []interface{}{}
		values = append(values,
			ucplog.LogFieldHTTPMethod, r.Method,
			ucplog.LogFieldRequestURL, r.URL.Path,
			ucplog.LogFieldContentLength, r.ContentLength,
		)
		logger := logr.FromContextOrDiscard(r.Context()).WithValues(values...)
		r = r.WithContext(logr.NewContext(r.Context(), logger))
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
