// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package radlogger

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/go-autorest/tracing"
)

var _ tracing.Tracer = (*Tracer)(nil)

// Tracer adds tracing support for autorest track 1 SDKs.
type Tracer struct {
}

func (t *Tracer) NewTransport(base *http.Transport) http.RoundTripper {
	return base
}

func (t *Tracer) StartSpan(ctx context.Context, name string) context.Context {
	spans := activeSpans(ctx)
	spans = append(spans, name)

	logger := GetLogger(ctx)
	if logger != nil {
		formatted := strings.Join(spans, "/")
		logger.Info(fmt.Sprintf("Starting Span: %s", formatted), LogFieldSpan, formatted)
	}

	return context.WithValue(ctx, key, spans)
}

func (t *Tracer) EndSpan(ctx context.Context, httpStatusCode int, err error) {
	spans := activeSpans(ctx)

	logger := GetLogger(ctx)
	if logger != nil {
		formatted := strings.Join(spans, "/")

		if err == nil {
			logger.Info(
				fmt.Sprintf("Ending Span: %s", formatted),
				LogFieldSpan, formatted,
				LogFieldStatusCode, httpStatusCode)
		} else {
			logger.Error(
				err,
				fmt.Sprintf("Ending Span with error: %s", formatted),
				LogFieldSpan, formatted,
				LogFieldStatusCode, httpStatusCode)
		}
	}
}

type spanKey string

var key = spanKey("Radius Tracer")

func activeSpans(ctx context.Context) []string {
	obj := ctx.Value(key)
	if obj != nil {
		return obj.([]string)
	}

	return []string{}
}
