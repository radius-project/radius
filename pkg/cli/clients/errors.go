// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"encoding/json"
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

// Is404Error returns true if the error is a 404 payload from an autorest operation.
//
// # Function Explanation
// 
//	Is404Error checks if the given error is a 404 error. It first checks if the error is an azcore ResponseError with a 
//	NotFound error code, and if not, it attempts to convert the error into a v20220315privatepreview ErrorResponse and 
//	checks if the error code is NotFound. If either of these checks are true, it returns true, otherwise it returns false. 
//	Callers of this function should use this to determine if the error they received was a 404 error.
func Is404Error(err error) bool {
	if err == nil {
		return false
	}

	// The error might already be an ResponseError
	responseError := &azcore.ResponseError{}
	if errors.As(err, &responseError) && responseError.ErrorCode == v1.CodeNotFound {
		return true
	} else if errors.As(err, &responseError) {
		return false
	}

	// OK so it's not an ResponseError, can we turn it into an ErrorResponse?
	errorResponse := v20220315privatepreview.ErrorResponse{}
	marshallErr := json.Unmarshal([]byte(err.Error()), &errorResponse)
	if marshallErr != nil {
		return false
	}

	if errorResponse.Error != nil && *errorResponse.Error.Code == v1.CodeNotFound {
		return true
	}

	return false
}
