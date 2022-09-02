// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

// See: https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/common-deployment-errors
//
// We get to define our own codes and document them, these are just examples, but consistency doesn't hurt.
const (
	// Used for generic validation errors.
	CodeInvalid = "BadRequest"

	// Used for internal/unclassified failures.
	CodeInternal = "Internal"

	// Used for CodeNotFound error.
	CodeNotFound = "NotFound"

	// Used for CodeConflict error.
	CodeConflict = "Conflict"

	// Used for CodeInvalidResourceType.
	CodeInvalidResourceType = "InvalidResourceType"

	// Used for CodeInvalidAuthenticationInfo.
	CodeInvalidAuthenticationInfo = "InvalidAuthenticationInfo"

	// Used for the cases when the precondition of a request fails.
	CodePreconditionFailed = "PreconditionFailed"

	// Used for CodeOperationCanceled.
	CodeOperationCanceled = "OperationCanceled"

	// Used for invalid api version parameter
	CodeInvalidApiVersionParameter = "InvalidApiVersionParameter"

	// Used for invalid request content.
	CodeInvalidRequestContent = "InvalidRequestContent"

	// Used for invalid object properties.
	CodeInvalidProperties = "InvalidProperties"

	// Used for failed invalid spec api validation.
	CodeHTTPRequestPayloadAPISpecValidationFailed = "HttpRequestPayloadAPISpecValidationFailed"
)
