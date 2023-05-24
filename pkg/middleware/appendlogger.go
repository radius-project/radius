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
