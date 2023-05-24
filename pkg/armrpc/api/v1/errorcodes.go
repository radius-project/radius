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
