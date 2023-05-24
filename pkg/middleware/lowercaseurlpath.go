/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package middleware

import (
	"net/http"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// LowercaseURLPath is the middelware to lowercase the incoming request url path.
func LowercaseURLPath(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// UCP/ARM populates "Referer" header in the request which can be used for FQDN of the resource.
		// This is the fallback setting "Referer" header to save the original URL for UCP scenario.
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-details.md#proxy-request-header-modifications

		if r.Header.Get(v1.RefererHeader) == "" {
			if r.URL.Host == "" {
				r.URL.Host = r.Host
			}
			r.Header.Set(v1.RefererHeader, r.URL.String())
		}

		r.URL.Path = strings.ToLower(r.URL.Path)
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
