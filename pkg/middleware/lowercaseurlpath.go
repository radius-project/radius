// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"
	"strings"
)

const (
	refererHeader = "Referer"
)

// LowercaseURLPath is the middelware to lowercase the incoming request url path.
func LowercaseURLPath(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// ARM populates "Referer" header in the request which can be used for FQDN of the resource.
		// This is the fallback setting "Referer" header to save the original URL for UCP scenario.
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-details.md#proxy-request-header-modifications
		if r.Header.Get(refererHeader) == "" {
			r.Header.Set(refererHeader, r.URL.String())
		}
		r.URL.Path = strings.ToLower(r.URL.Path)
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
