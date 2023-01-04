// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// Append logger values to the context based on the Resource ID (if present).
func AppendLogValues(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		id, err := resources.Parse(r.URL.Path)
		if err != nil {
			// This just means the request is for an ARM resource. Not an error.
			h.ServeHTTP(w, r)
			return
		}

		values := []interface{}{}
		values = append(values, radlogger.LogFieldResourceID, id.String())
		values = append(values, radlogger.LogFieldRootScope, id.RootScope())
		values = append(values, radlogger.LogFieldRoutingScope, id.RoutingScope())
		values = append(values, radlogger.LogFieldResourceType, id.Type())
		values = append(values, radlogger.LogFieldResourceName, id.Name())

		//If present, log the correlation id
		values = append(values, radlogger.LogCorrelationID, r.Header.Get(radlogger.LogCorrelationID))

		// TODO: populate correlation id and w3c trace parent id - https://github.com/project-radius/core-team/issues/53

		r = r.WithContext(radlogger.WrapLogContext(r.Context(), values...))
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
