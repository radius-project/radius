// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	UCP_API     string = "ucp.api"
	UCP_REQ_URI string = "ucp.request.uri"
)

func HTTPTracingMiddleWare(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		tracer := otel.Tracer("middleware")
		ctx, span := tracer.Start(r.Context(), r.URL.Path)
		defer span.End()
		r = r.WithContext(ctx)
		var spanAttrKey attribute.Key
		span.AddEvent(fmt.Sprintf("UCP Recieved request %s", r.URL.Path))

		for key, value := range r.Header {
			values := strings.Join(value[:], ",")
			spanAttrKey = attribute.Key(key)
			span.SetAttributes(spanAttrKey.String(values))

		}

		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
