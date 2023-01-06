// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

// Field names for structured logging
const (
	//HTTP status code of response from downstream as seen by UCP
	LogHTTPStatusCode string = "statusCode"

	//ID of a resource provider plane
	LogFieldPlaneID string = "PlaneID"

	//Path of the request URL
	LogFieldRequestPath string = "Path"

	//Kind of Plane
	LogFieldPlaneKind string = "PlaneKind"

	//Scheme of HTTP request
	LogFieldHTTPScheme string = "HTTPScheme"

	//URL to which this request will be proxied to
	LogFieldPlaneURL string = "ProxyURL"

	//Provider fulfilling the request
	LogFieldProvider string = "Provider"

	//UCP resource group
	LogFieldResourceGroup string = "UCPResourceGroup"

	//HTTP request method of request recieved by UCP
	LogFieldHTTPMethod string = "HttpMethod"

	//HTTP request received by UCP from upstreamgit upstea
	LogFieldRequestURL string = "RequestURL"

	//HTTP request/ response content-length
	LogFieldContentLength string = "ContentLength"

	//UCP host name
	LogFieldUCPHost string = "UCPHost"
)
