// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/radlogger"
)

// Append logger values to the context based on the Resource ID (if present).
func AppendLogValues(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		id, err := azresources.Parse(r.URL.Path)
		if err != nil {
			// This just means the request is for an ARM resource. Not an error.
			h.ServeHTTP(w, r)
			return
		}

		values := []interface{}{}
		values = append(values, radlogger.LogFieldResourceID, id.ID)

		// TODO: populate correlation id and w3c trace parent id - https://github.com/project-radius/core-team/issues/53

		// values = append(values, radlogger.LogFieldSubscriptionID, id.SubscriptionID)
		// values = append(values, radlogger.LogFieldResourceGroup, id.ResourceGroup)
		// values = append(values, radlogger.LogFieldResourceType, id.Type())
		// values = append(values, radlogger.LogFieldResourceName, id.QualifiedName())

		r = r.WithContext(radlogger.WrapLogContext(r.Context(), values...))
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
