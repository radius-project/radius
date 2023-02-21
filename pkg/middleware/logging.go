// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
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

// UseLogValues appends logging values to the context based on the request.
func UseLogValues(h http.Handler, basePath string) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		values := []any{}

		attr := map[attribute.Key]string{}
		attr = AddAttribute(semconv.ServiceNameKey, ucplog.UCPServiceName, attr)
		attr = AddAttribute(semconv.ServiceVersionKey, version.Channel(), attr)

		id, err := resources.Parse(r.URL.Path)
		if err == nil {
			attr = AddAttribute(attribute.Key(ucplog.LogFieldResourceID), id.String(), attr)
		}

		host, _ := os.Hostname()
		attr = AddAttribute(semconv.HostNameKey, host, attr)

		clientIP := r.Header.Get(ucplog.HttpXForwardedFor)
		if clientIP == "" {
			remote := r.RemoteAddr
			clientIP, _, _ = net.SplitHostPort(remote)
		}
		attr = AddAttribute(semconv.HTTPClientIPKey, clientIP, attr)
		attr = AddAttribute(semconv.HTTPUserAgentKey, r.Header.Get(ucplog.HttpUserAgent), attr)
		attr = AddAttribute(attribute.Key(ucplog.LogFieldCorrelationID), r.Header.Get(ucplog.HttpCorrelationId), attr)
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

		logger := logr.FromContextOrDiscard(r.Context()).WithValues(values...)
		r = r.WithContext(logr.NewContext(r.Context(), logger))
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

// GetRelativePath trims the prefix basePath from path
func GetRelativePath(basePath string, path string) string {
	trimmedPath := strings.TrimPrefix(path, basePath)
	return trimmedPath
}

// Add an optional attribute to the log message
func AddAttribute(attrKey attribute.Key, value string, m map[attribute.Key]string) map[attribute.Key]string {
	if value != "" {
		m[attrKey] = value
	}
	return m
}

// Add a mandatory field to the log message
func AddLogValue(key string, value any, values []any) []any {
	if key == "" || value == "" {
		return values
	}
	return append(values, key, value)
}
