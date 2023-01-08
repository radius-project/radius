// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

// Field names for structured logging
const (
	// LogHTTPStatusCode represents the HTTP status code of response from downstream as seen by UCP.
	LogHTTPStatusCode string = "StatusCode"

	// LogFieldPlaneID represents the ID of a resource provider plane.
	LogFieldPlaneID string = "PlaneID"

	// LogFieldRequestPath represents the path of the request URL.
	LogFieldRequestPath string = "Path"

	// LogFieldPlaneKind represents the kind of plane.
	LogFieldPlaneKind string = "PlaneKind"

	// LogFieldHTTPScheme represents the scheme of HTTP request.
	LogFieldHTTPScheme string = "HTTPScheme"

	// LogFieldPlaneURL represents the URL to which this request will be proxied to.
	LogFieldPlaneURL string = "ProxyURL"

	// LogFieldProvider represents the Resource Provider fulfilling the request.
	LogFieldProvider string = "Provider"

	// LogFieldResourceGroup represents the UCP resource group.
	LogFieldResourceGroup string = "UCPResourceGroup"

	// LogFieldHTTPMethod represents the HTTP request method of request recieved by UCP from client.
	LogFieldHTTPMethod string = "HttpMethod"

	// LogFieldRequestURL represents the HTTP request URL of request received by UCP from client.
	LogFieldRequestURL string = "RequestURL"

	// LogFieldContentLength represents the content-length of the HTTP request/ response received by UCP.
	LogFieldContentLength string = "ContentLength"

	// LogFieldUCPHost represents the UCP server host name.
	LogFieldUCPHost string = "UCPHost"
)
