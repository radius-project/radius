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

package ucplog

const (
	LoggerName  string = "ucplogger"
	ServiceName string = "ucp"
)

// Field names for structured logging
const (
	// LogFieldHostName represents the current hostname.
	LogFieldHostname string = "hostName"

	// LogFieldVersion represents the version of service.
	LogFieldVersion string = "version"

	// LogFieldResourceID represents the Radius Resource ID.
	LogFieldResourceID string = "resourceId"

	// LogFieldTargetResourceID represents the resource ID of a non-Radius resource. eg: an output resource.
	LogFieldTargetResourceID string = "targetResourceID"

	// LogFieldCorrelationID represents the X-Correlation-ID that may be present in the incoming request.
	LogFieldCorrelationID string = "correlationId"

	// LogFieldServiceName represents the name of the service generating the log entry
	LogFieldServiceName string = "serviceName"

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
