// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"
)

// RemoveRemoteAddr is the middleware to remove remoteaddr to avoid high cardinality in metrics.
// This is a temporary workaround until opentelemetry-go fixes the issue - https://github.com/open-telemetry/opentelemetry-go-contrib/issues/3765
func RemoveRemoteAddr(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = ""
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
