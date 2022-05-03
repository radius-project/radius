// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

// LowercaseURLPath is the middelware to lowercase the incoming request url path.
func MetricsInterceptor(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// promMetricsClient, _ = mp.NewPrometheusMetricsClient()
		fmt.Printf(mux.CurrentRoute(r).GetName())
		fmt.Printf("metrics middleware")
		
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}