/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

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
