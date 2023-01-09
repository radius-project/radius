// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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

		values := []any{}
		values = append(values, logging.LogFieldResourceID, id.String())

		// TODO: populate correlation id and w3c trace parent id - https://github.com/project-radius/core-team/issues/53

		// values = append(values, logging.LogFieldSubscriptionID, id.SubscriptionID)
		// values = append(values, logging.LogFieldResourceGroup, id.ResourceGroup)
		// values = append(values, logging.LogFieldResourceType, id.Type())
		// values = append(values, logging.LogFieldResourceName, id.QualifiedName())

		r = r.WithContext(ucplog.WrapLogContext(r.Context(), values...))
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
