// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/version"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const invalidTrace string = "00-00000000000000000000000000000000-0000000000000000-00"

// UseLogValues appends logging values to the context based on the request.
func UseLogValues(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		values := []any{}

		attr := map[attribute.Key]string{}
		attr[semconv.ServiceNameKey] = ucplog.UCPServiceName
		attr[semconv.ServiceVersionKey] = version.Channel()
		attr[semconv.HTTPMethodKey] = r.Method
		attr[semconv.HTTPTargetKey] = r.URL.RequestURI()
		attr[semconv.HTTPRequestContentLengthKey] = fmt.Sprint(r.ContentLength)

		id, err := resources.Parse(r.URL.Path)
		if err == nil {
			attr[attribute.Key(ucplog.LogFieldResourceID)] = id.String()
		}

		host, _ := os.Hostname()
		attr = AddAttribute(semconv.HostNameKey, host, attr)

		clientIP := r.Header.Get(ucplog.HttpXForwardedForHeader)
		if clientIP == "" {
			remote := r.RemoteAddr
			clientIP, _, _ = net.SplitHostPort(remote)
		}
		attr = AddAttribute(semconv.HTTPClientIPKey, clientIP, attr)

		if len(attr) > 0 {
			values = append(values,
				ucplog.LogFieldAttributes, attr,
			)
		}

		sc := trace.SpanFromContext(r.Context())
		values = append(values,
			ucplog.LogFieldSpanId, sc.SpanContext().SpanID().String(),
			ucplog.LogFieldTraceId, sc.SpanContext().TraceID().String(),
		)

		values = AddLogValue(ucplog.LogFieldCorrelationID, r.Header.Get(ucplog.LogFieldCorrelationID), values)

		logger := logr.FromContextOrDiscard(r.Context()).WithValues(values...)
		r = r.WithContext(logr.NewContext(r.Context(), logger))
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func GetRelativePath(basePath string, path string) string {
	trimmedPath := strings.TrimPrefix(path, basePath)
	return trimmedPath
}

func AddAttribute(attrKey attribute.Key, value string, m map[attribute.Key]string) map[attribute.Key]string {
	if value != "" {
		m[attrKey] = value
	}
	return m
}

func AddLogValue(key string, value any, values []any) []any {
	if key == "" || value == "" {
		return values
	}
	return append(values, key, value)
}

func IsValidTraceID(w3cTraceID string) bool {
	return w3cTraceID != "" && w3cTraceID != invalidTrace
}
