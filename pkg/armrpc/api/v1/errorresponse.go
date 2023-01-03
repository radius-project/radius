// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

// see : https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-details.md#error-response-content

// ErrorResponse represents an error HTTP response as defined by the ARM API.
type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

// ErrorDetails represents an error as defined by the ARM API.
type ErrorDetails struct {
	Code           string                `json:"code"`
	Message        string                `json:"message"`
	Target         string                `json:"target,omitempty"`
	AdditionalInfo []ErrorAdditionalInfo `json:"additionalInfo,omitempty"`
	Details        []ErrorDetails        `json:"details,omitempty"`
}

// ErrorAdditionalInfo represents abritrary additional information as part of an error as defined by the ARM API.
type ErrorAdditionalInfo struct {
	Type string         `json:"type"`
	Info map[string]any `json:"info"`
}
