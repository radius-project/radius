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

package clients

import (
	"encoding/json"
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
)

// Is404Error returns true if the error is a 404 payload from an autorest operation.
//

// "Is404Error" checks if the given error is a 404 error by checking if it is a ResponseError with an ErrorCode of
// "NotFound" or an ErrorResponse with an Error Code of "NotFound".
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
	errorResponse := v20231001preview.ErrorResponse{}
	marshallErr := json.Unmarshal([]byte(err.Error()), &errorResponse)
	if marshallErr != nil {
		return false
	}

	if errorResponse.Error != nil && *errorResponse.Error.Code == v1.CodeNotFound {
		return true
	}

	return false
}
