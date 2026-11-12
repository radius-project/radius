/*
Copyright 2026 The Radius Authors.

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

package token

import (
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

// Error is a token verification failure with an authorization error code. Code maps to
// an HTTP status through authz.StatusForCode. Reason never contains the token.
type Error struct {
	// Code is v1.CodeInvalidAuthenticationInfo or v1.CodeAuthorizationUnavailable.
	Code string

	// Reason describes the failure.
	Reason string

	// Err is the underlying error, if any.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Reason, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Reason)
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

func invalid(reason string, err error) *Error {
	return &Error{Code: v1.CodeInvalidAuthenticationInfo, Reason: reason, Err: err}
}

func unavailable(reason string, err error) *Error {
	return &Error{Code: v1.CodeAuthorizationUnavailable, Reason: reason, Err: err}
}
