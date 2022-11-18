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
func (e *ErrUnsupportedModel) Error() string {
	return "invalid model for prompt operation"
}

// Is checks if the error provided is of type ErrUnsupportedModel.
func (e *ErrUnsupportedModel) Is(target error) bool {
	t, ok := target.(*ErrUnsupportedModel)
	if !ok {
		return false
	}

	return (e.Message == t.Message || t.Message == "")
}
