// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"
)

// RemoveRemoteAddr is the middelware to remove remoteaddr to avoid high cardinality in metrics.
func RemoveRemoteAddr(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = ""
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
