// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

const (
	LoggerName  string = "ucplogger"
	ServiceName string = "ucp"
)

// Field names for structured logging
const (
	// LogFieldUCPHost represents the Radius Resource ID.
	LogFieldResourceID string = "resourceId"

	// LogFieldCorrelationID represents the X-Correlation-ID that may be present in the incoming request.
	LogFieldCorrelationID string = "correlationId"

	// LogFieldServiceID represents the name of the service generating the log entry
	LogFieldServiceID string = "serviceId"

	// LogFieldAttributes represents the optional attributes associated with a log message
	LogFieldAttributes string = "attributes"

	// LogFieldTraceId represents the traceId retrieved from traceparent header of the current HTTP request
	LogFieldTraceId string = "traceId"

	// LogFieldSpanId represents the spanId retrieved from traceparent header of current HTTP request
	LogFieldSpanId string = "spanId"

	// HttpXForwardedFor represents the x-forwarded-for HTTP header
	HttpXForwardedFor string = "x-forwarded-for"

	// HttpCorrelationId represents the x-forwarded-for HTTP header
	HttpCorrelationId string = "x-correlation-id"

	// HttpUserAgent represents the user-agent HTTP header
	HttpUserAgent string = "user-agent"
)
