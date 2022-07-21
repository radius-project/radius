// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	// RefererHeader is the full URI that the client connected to (which will be different than the RP URI, since it will have the public
	// hostname instead of the RP hostname). This value can be used in generating FQDN for Location headers or other requests since RPs
	// should not reference their endpoint name.
	RefererHeader = "Referer"

	XRawResourcePathHeader = "X-Raw-ResourcePath"
)

// LowercaseURLPath is the middelware to lowercase the incoming request url path.
func LowercaseURLPath(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// ARM populates "Referer" header in the request which can be used for FQDN of the resource.
		// This is the fallback setting "Referer" header to save the original URL for UCP scenario.
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-details.md#proxy-request-header-modifications
		if r.Header.Get(RefererHeader) == "" {
			r.Header.Set(RefererHeader, r.URL.String())
		}
		// Preserve the original route path before lowercasing the path.
		r.Header.Set(XRawResourcePathHeader, r.URL.Path)
		r.URL.Path = strings.ToLower(r.URL.Path)
		fmt.Printf("!!!!! incoming url: %s \n", r.URL.String())
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
