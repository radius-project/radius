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
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

const (
	fakeServerNotFoundResponse = "unexpected status code 404. acceptable values are http.StatusOK"
)

// errorResponse is the standard ARM error envelope used to decode error
// payloads that are not already typed as an azcore.ResponseError.
type errorResponse struct {
	Error *errorDetail `json:"error,omitempty"`
}

type errorDetail struct {
	Code    *string `json:"code,omitempty"`
	Message *string `json:"message,omitempty"`
}

// Is404Error returns true if the error is a 404 payload from an autorest operation.
//

// "Is404Error" checks if the given error is a 404 error by checking if it is one of:
// a ResponseError with an ErrorCode of "NotFound", or
// a ResponseError with a StatusCode of 404, or
// an ErrorResponse with an Error Code of "NotFound".
func Is404Error(err error) bool {
	if err == nil {
		return false
	}

	// NotFound Response from Fake Server - used for testing
	if strings.Contains(err.Error(), fakeServerNotFoundResponse) {
		return true
	}

	// The error might already be an ResponseError
	responseError := &azcore.ResponseError{}
	if errors.As(err, &responseError) && responseError.ErrorCode == v1.CodeNotFound || responseError.StatusCode == http.StatusNotFound {
		return true
	} else if errors.As(err, &responseError) {
		return false
	}

	// OK so it's not an ResponseError, can we turn it into an ErrorResponse?
	errorResponse := errorResponse{}
	marshallErr := json.Unmarshal([]byte(err.Error()), &errorResponse)
	if marshallErr != nil {
		return false
	}

	if errorResponse.Error != nil && *errorResponse.Error.Code == v1.CodeNotFound {
		return true
	}

	return false
}

// IsNamespaceAlreadyInUseError reports whether err is the server's "the requested Kubernetes
// namespace is already claimed by another environment" response.
//
// It matches on the specific error code rather than on the 409 status, because the server returns
// Conflict for unrelated reasons too (for example, a resource that is still provisioning), and
// reporting those as a namespace collision would misdiagnose them.
func IsNamespaceAlreadyInUseError(err error) bool {
	return hasErrorCode(err, v1.CodeNamespaceAlreadyInUse)
}

// NamespaceAlreadyInUseMessage returns the server's message for a namespace conflict, or an empty
// string when err is not a namespace conflict or the message cannot be recovered. The server names
// the environment that currently owns the namespace, which the CLI cannot determine on its own.
func NamespaceAlreadyInUseMessage(err error) string {
	if !IsNamespaceAlreadyInUseError(err) {
		return ""
	}

	detail := decodeErrorDetail(err)
	if detail == nil || detail.Message == nil {
		return ""
	}

	return *detail.Message
}

// hasErrorCode reports whether err carries the given ARM error code, whether it arrives as a typed
// azcore.ResponseError or as a raw JSON error envelope.
func hasErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}

	responseError := &azcore.ResponseError{}
	if errors.As(err, &responseError) {
		return responseError.ErrorCode == code
	}

	detail := decodeErrorDetail(err)

	return detail != nil && detail.Code != nil && *detail.Code == code
}

// decodeErrorDetail recovers the ARM error envelope from err, returning nil when it cannot be
// found. A raw envelope error is JSON in its entirety, while an azcore.ResponseError renders the
// response body embedded in surrounding diagnostic text, so decoding starts at the first brace and
// ignores whatever follows the first complete JSON value.
func decodeErrorDetail(err error) *errorDetail {
	if err == nil {
		return nil
	}

	text := err.Error()
	start := strings.Index(text, "{")
	if start < 0 {
		return nil
	}

	response := errorResponse{}
	if decodeErr := json.NewDecoder(strings.NewReader(text[start:])).Decode(&response); decodeErr != nil {
		return nil
	}

	return response.Error
}
