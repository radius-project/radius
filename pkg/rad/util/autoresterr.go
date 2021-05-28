// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package util

import (
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
)

// ExtractServiceError returns an azure.ServiceError if the error contains a service error payload.
func ExtractServiceError(err error) (azure.ServiceError, bool) {
	if err == nil {
		return azure.ServiceError{}, false
	}

	if service, ok := err.(*azure.ServiceError); ok {
		return *service, true
	}

	if detailed, ok := err.(*autorest.DetailedError); ok {
		return ExtractServiceError(detailed.Original)
	}

	if detailed, ok := err.(autorest.DetailedError); ok {
		return ExtractServiceError(detailed.Original)
	}

	if request, ok := err.(*azure.RequestError); ok && request.ServiceError != nil {
		return *request.ServiceError, true
	}

	return azure.ServiceError{}, false
}

// ExtractDetailedError returns an autorest.DetailedError if the error contains a detailed error payload.
func ExtractDetailedError(err error) (autorest.DetailedError, bool) {
	if err == nil {
		return autorest.DetailedError{}, false
	}

	if detailed, ok := err.(*autorest.DetailedError); ok {
		return *detailed, ok
	}

	if detailed, ok := err.(autorest.DetailedError); ok {
		return detailed, ok
	}

	return autorest.DetailedError{}, false
}

// IsAutorest404Error returns true if the error is a 404 payload from an autorest operation.
func IsAutorest404Error(err error) bool {
	detailed, ok := ExtractDetailedError(err)
	if !ok {
		return false
	}

	return detailed.Response != nil && detailed.Response.StatusCode == 404
}
