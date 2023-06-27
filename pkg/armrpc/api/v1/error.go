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
//	ErrModelConversion's Error() function returns a string with a message that describes an error related to a property name
//	 and its valid value. It is useful for callers of this function to understand what went wrong when an error is returned.
func (e *ErrModelConversion) Error() string {
	return fmt.Sprintf("%s must be %s.", e.PropertyName, e.ValidValue)
}

// # Function Explanation
// 
//	ErrModelConversion's Is() function checks if the given error is of type ErrModelConversion, and returns a boolean value 
//	indicating the result. This allows callers to handle errors of this type specifically.
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
//	ErrClientRP's Error() function returns a string containing the error code and message, which can be used by callers to 
//	handle errors.
func (r *ErrClientRP) Error() string {
	return fmt.Sprintf("code %v: err %v", r.Code, r.Message)
}
// # Function Explanation
// 
//	ErrClientRP's Is() function checks if the given error is of type ErrClientRP and returns a boolean value accordingly, 
//	allowing callers to handle errors appropriately.
func (e *ErrClientRP) Is(target error) bool {
	_, ok := target.(*ErrClientRP)
	return ok
}

// # Function Explanation
// 
//	NewClientErrInvalidRequest creates a new ErrClientRP error with a given message and sets the Code to CodeInvalid, 
//	allowing callers to handle the error accordingly.
func NewClientErrInvalidRequest(message string) *ErrClientRP {
	err := new(ErrClientRP)
	err.Message = message
	err.Code = CodeInvalid
	return err
}
