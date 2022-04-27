// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"
	"strings"
)

// LowercaseURLPath is the middelware to lowercase the incoming request url path.
func LowercaseURLPath(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.ToLower(r.URL.Path)
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
