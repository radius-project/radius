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
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

func TestIs404Error(t *testing.T) {
	var err error

	// Test with a ResponseError with an ErrorCode of "NotFound"
	err = &azcore.ResponseError{ErrorCode: v1.CodeNotFound}
	if !Is404Error(err) {
		t.Errorf("Expected Is404Error to return true for ResponseError with ErrorCode of 'NotFound', but it returned false")
	}

	// Test with a ResponseError with a StatusCode of 404
	err = &azcore.ResponseError{StatusCode: http.StatusNotFound}
	if !Is404Error(err) {
		t.Errorf("Expected Is404Error to return true for ResponseError with StatusCode of 404, but it returned false")
	}

	// Test with an ErrorResponse with an Error Code of "NotFound"
	err = errors.New(`{"error": {"code": "NotFound"}}`)
	if !Is404Error(err) {
		t.Errorf("Expected Is404Error to return true for ErrorResponse with Error Code of 'NotFound', but it returned false")
	}

	// Test with an ErrorResponse with a different Error Code
	err = errors.New(`{"error": {"code": "SomeOtherCode"}}`)
	if Is404Error(err) {
		t.Errorf("Expected Is404Error to return false for ErrorResponse with Error Code of 'SomeOtherCode', but it returned true")
	}

	// Test with a different error type
	err = errors.New("Some other error")
	if Is404Error(err) {
		t.Errorf("Expected Is404Error to return false for error of type %T, but it returned true", err)
	}

	// Test with a nil error
	if Is404Error(nil) {
		t.Errorf("Expected Is404Error to return false for nil error, but it returned true")
	}

	// Test with a fake server not found response
	err = errors.New(fakeServerNotFoundResponse)
	if !Is404Error(err) {
		t.Errorf("Expected Is404Error to return true for fake server not found response, but it returned false")
	}
}
