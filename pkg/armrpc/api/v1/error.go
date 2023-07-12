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

package v1

import (
	"errors"
	"fmt"
)

var (
	ErrUnsupportedAPIVersion = errors.New("unsupported api-version")
	// ErrInvalidModelConversion is the error when converting model is invalid.
	ErrInvalidModelConversion = errors.New("invalid model conversion")
)

// ErrModelConversion represents an invalid property error.
type ErrModelConversion struct {
	PropertyName string
	ValidValue   string
}

// # Function Explanation
//
// Error returns an error string describing the property name and valid value.
func (e *ErrModelConversion) Error() string {
	return fmt.Sprintf("%s must be %s.", e.PropertyName, e.ValidValue)
}

// # Function Explanation
//
// Is checks if the target error is of type ErrModelConversion.
func (e *ErrModelConversion) Is(target error) bool {
	_, ok := target.(*ErrModelConversion)
	return ok
}

type ErrClientRP struct {
	Code    string
	Message string
}

// # Function Explanation
//
// Error returns an error string describing the error code and message.
func (r *ErrClientRP) Error() string {
	return fmt.Sprintf("code %v: err %v", r.Code, r.Message)
}

// # Function Explanation
//
// Is checks if the target error is the type of ErrClientRP and returns true if it is the same error type.
func (e *ErrClientRP) Is(target error) bool {
	_, ok := target.(*ErrClientRP)
	return ok
}

// # Function Explanation
//
// NewClientErrInvalidRequest creates a new ErrClientRP error with a given message and sets the code to CodeInvalid.
func NewClientErrInvalidRequest(message string) *ErrClientRP {
	err := new(ErrClientRP)
	err.Message = message
	err.Code = CodeInvalid
	return err
}
