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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// OverrideResourceIDMiddleware is a middleware that tweaks the resource ID of the request.
//
// This is useful for URLs that don't follow the usual ResourceID pattern. We still want these
// URLs to be handled by our data storage and telemetry systems in the same way.
//
// For example a request like:
//
//	GET /planes/radius/local/providers -> ResourceID: /planes/radius/local/providers/System.Resources/resourceProviders
func OverrideResourceID(override func(req *http.Request) (resources.ID, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// This handler will get the resource ID and update the stored request to refer to it.
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, err := override(req)
			if err != nil {
				logger := ucplog.FromContextOrDiscard(req.Context())
				logger.Error(err, "failed to override resource ID")
				next.ServeHTTP(w, req)
				return
			}

			// Update the request context with the new resource ID.
			armCtx := v1.ARMRequestContextFromContext(req.Context())
			if armCtx != nil {
				armCtx.ResourceID = id
				*req = *req.WithContext(v1.WithARMRequestContext(req.Context(), armCtx))
			}

			next.ServeHTTP(w, req)
		})
	}
}
