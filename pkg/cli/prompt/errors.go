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

package prompt

var _ error = (*ErrUnsupportedModel)(nil)

// ErrUnsupportedModel represents error when invalid bubble tea model is used for operations.
type ErrUnsupportedModel struct {
	Message string
}

// ErrUnsupportedModel returns the error message.
//
// # Function Explanation
// 
//	ErrUnsupportedModel is an error returned by the ErrUnsupportedModel function when an invalid model is used for a prompt 
//	operation. It provides useful information to the caller about the error.
func (e *ErrUnsupportedModel) Error() string {
	return "invalid model for prompt operation"
}

// Is checks if the error provided is of type ErrUnsupportedModel.
//
// # Function Explanation
// 
//	ErrUnsupportedModel's Is() function checks if the given error is of the same type and has the same message as the 
//	original error, returning a boolean value. This allows callers to check if the error they received is of the same type 
//	as the one they expected.
func (e *ErrUnsupportedModel) Is(target error) bool {
	t, ok := target.(*ErrUnsupportedModel)
	if !ok {
		return false
	}

	return (e.Message == t.Message || t.Message == "")
}
