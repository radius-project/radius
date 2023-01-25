// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

const (
	UCPLoggerName string = "ucp"
)

// Field names for structured logging
const (
	// LogHTTPStatusCode represents the HTTP status code of response from downstream as seen by UCP.
	LogHTTPStatusCode string = "ResponseStatusCode"

	// LogFieldRequestPath represents the path of the request URL.
	LogFieldRequestPath string = "RequestPath"

	// LogFieldHTTPScheme represents the scheme of HTTP request.
	LogFieldHTTPScheme string = "RequestScheme"

	// LogFieldPlaneURL represents the URL to which this request will be proxied to.
	LogFieldPlaneURL string = "ProxyURL"

	// LogFieldResourceGroup represents the UCP resource group.
	LogFieldResourceGroup string = "UCPResourceGroup"

	// LogFieldHTTPMethod represents the HTTP request method of request recieved by UCP from client.
	LogFieldHTTPMethod string = "RequestMethod"

	// LogFieldRequestURL represents the HTTP request URL of request received by UCP from client.
	LogFieldRequestURL string = "RequestURL"

	// LogFieldContentLength represents the content-length of the HTTP request/ response received by UCP.
	LogFieldContentLength string = "ContentLength"

	// LogFieldUCPHost represents the UCP server host name.
	LogFieldUCPHost string = "UCPHost"

	// LogFieldUCPHost represents the Resource ID.
	LogFieldResourceID string = "ResourceID"

	// LogFieldCorrelationID represents the X-Correlation-ID that may be present in the incoming request.
	LogFieldCorrelationID string = "X-Correlation-ID"
)
