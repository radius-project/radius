// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

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

// Is404Error returns true if the error is a 404 payload from an autorest operation.
func Is404Error(err error) bool {
	if detailed, ok := ExtractDetailedError(err); ok && detailed.Response != nil && detailed.Response.StatusCode == 404 {
		return true
	} else if serviceErr, ok := ExtractServiceError(detailed.Original); ok && (serviceErr.Code == "ResourceNotFound" || serviceErr.Code == "NotFound") {
		return true
	}

	return false
}
