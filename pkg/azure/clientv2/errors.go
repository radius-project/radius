// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"errors"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// Is404Error returns true if the error is a 404 payload from an autorest operation.
func Is404Error(err error) bool {
	respErr, ok := ExtractResponseError(err)
	if !ok {
		return false
	}

	if respErr != nil && respErr.StatusCode == http.StatusNotFound {
		return true
	}

	return false
}

// ExtractResponseError extracts the ResponseError from the error.
// Returns true if the error is a ResponseError.
func ExtractResponseError(err error) (*azcore.ResponseError, bool) {
	if err == nil {
		return nil, false
	}

	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return respErr, true
	}

	return nil, false
}
