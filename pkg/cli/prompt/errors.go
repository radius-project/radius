// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
