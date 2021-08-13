// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armerrors

// See: https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/common-deployment-errors
//
// We get to define our own codes and document them, these are just examples, but consistency doesn't hurt.
const (
	// Used for generic validation errors.
	Invalid = "BadRequest"

	// Used for internal/unclassified failures.
	Internal = "Internal"

	// Used for NotFound error.
	NotFound = "NotFound"

	// Used for Conflict error.
	Conflict = "Conflict"
)

// see : https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-details.md#error-response-content

// ErrorResponse represents an error HTTP response as defined by the ARM API.
type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

// ErrorDetails represents an error as defined by the ARM API.
type ErrorDetails struct {
	Code           string                `json:"code"`
	Message        string                `json:"message"`
	Target         string                `json:"target"`
	AdditionalInfo []ErrorAdditionalInfo `json:"additionalInfo,omitempty"`
	Details        []ErrorDetails        `json:"details,omitempty"`
}

// ErrorAdditionalInfo represents abritrary additional information as part of an error as defined by the ARM API.
type ErrorAdditionalInfo struct {
	Type string                 `json:"type"`
	Info map[string]interface{} `json:"info"`
}
