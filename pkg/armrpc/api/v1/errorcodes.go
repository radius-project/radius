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

	// Used when a dependency to carry out current operation is missing.
	CodeDependencyMissing = "DependencyMissing"

	// Used for CodeNotFound error.
	CodeNotFound = "NotFound"

	// Used for CodeConflict error.
	CodeConflict = "Conflict"

	// Used when the Kubernetes namespace requested by an environment is already claimed by
	// another environment. This is distinct from CodeConflict so that clients can recognize
	// the namespace collision specifically rather than treating every 409 the same way.
	CodeNamespaceAlreadyInUse = "NamespaceAlreadyInUse"

	// Used when a request attempts to change or clear the Kubernetes namespace of an environment
	// that already has one. The namespace is immutable once established, because changing it
	// would orphan resources already deployed into the previous namespace.
	CodeNamespaceImmutable = "NamespaceImmutable"

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

	// Used for invalid plane type.
	CodeInvalidPlaneType = "InvalidPlaneType"

	// Used for failed invalid spec api validation.
	CodeHTTPRequestPayloadAPISpecValidationFailed = "HttpRequestPayloadAPISpecValidationFailed"
)

// Authorization error codes. See pkg/authz for the HTTP status of each code.
const (
	// Used when the user does not have permission for the requested action on the target (403).
	CodeAuthorizationFailed = "AuthorizationFailed"

	// Used when the execution record is missing, closed, revoked, or expired (403).
	CodeExecutionRecordNotActive = "ExecutionRecordNotActive"

	// Used when the request is outside the execution record's approved actions or targets (403).
	CodeGrantScopeExceeded = "GrantScopeExceeded"

	// Used when the caller is not the operation's assigned component, or the operation is already final (403).
	CodeOperationNotAssigned = "OperationNotAssigned"

	// Used when the work's inputs do not match the operation's approved input hash (409).
	CodeOperationInputMismatch = "OperationInputMismatch"

	// Used when queued work waited longer than the execution record's queue wait limit (403).
	CodeQueueWaitLimitExceeded = "QueueWaitLimitExceeded"

	// Used when the credential broker refuses a token because the requested scope is outside
	// the environment or the execution record (403).
	CodeCredentialIssuanceDenied = "CredentialIssuanceDenied"

	// Used when a Kubernetes admission control rejects an application workload (403).
	CodeAdmissionPolicyDenied = "AdmissionPolicyDenied"

	// Used when Radius could not read the user's permissions or the execution record, so it
	// refused rather than guessed (503).
	CodeAuthorizationUnavailable = "AuthorizationUnavailable"
)
