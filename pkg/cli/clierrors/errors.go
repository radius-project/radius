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

import "fmt"

// IsFriendlyError returns true if the error should be handled gracefully by the CLI.
func IsFriendlyError(err error) bool {
	friendly, ok := err.(FriendlyError)
	return ok && friendly.IsFriendlyError()
}

// Message returns a new ErrorMessage with the given message. The message can be formatted with args.
func Message(message string, args ...any) *ErrorMessage {
	return &ErrorMessage{Message: fmt.Sprintf(message, args...)}
}

// Message returns a new ErrorMessage with the given message and cause. The message can be formatted with args.
func MessageWithCause(cause error, message string, args ...any) *ErrorMessage {
	return &ErrorMessage{Cause: cause, Message: fmt.Sprintf(message, args...)}
}
