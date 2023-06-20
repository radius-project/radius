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

package clierrors

// FriendlyError defines an interface for errors that should be gracefully handled by the CLI and
// display a friendly error message to the user.
type FriendlyError interface {
	error

	// IsFriendlyError returns true if the error should be handled gracefully by the CLI.
	IsFriendlyError() bool
}

var _ FriendlyError = &ErrorMessage{}

// ErrorMessage represents a basic error message that can be returned by the CLI.
type ErrorMessage struct {
	// Message is the error message.
	Message string

	// Cause is the root cause of the error. If provided it will be included in the message displayed to users.
	Cause error
}

// Error returns the error message for the error.
func (e *ErrorMessage) Error() string {
	if e.Cause == nil {
		return e.Message
	}

	return e.Message + " Cause: " + e.Cause.Error() + "."
}

// IsFriendlyError returns true for ErrorMessage. These errors are always handled gracefully by the CLI.
func (*ErrorMessage) IsFriendlyError() bool {
	return true
}

// Unwrap returns the cause of the error.
func (e *ErrorMessage) Unwrap() error {
	return e.Cause
}
