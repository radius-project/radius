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

package store

import "fmt"

var _ error = (*ErrInvalid)(nil)

type ErrInvalid struct {
	Message string
}

// # Function Explanation
//
// Error returns a string representation of the error.
func (e *ErrInvalid) Error() string {
	return e.Message
}

// # Function Explanation
//
// Is checks if the target error is of type ErrInvalid and if the message of the target error is equal to the
// message of the ErrInvalid instance or is an empty string.
func (e *ErrInvalid) Is(target error) bool {
	t, ok := target.(*ErrInvalid)
	if !ok {
		return false
	}

	return (e.Message == t.Message || t.Message == "")
}

type ErrNotFound struct {
	// ID of the resource that was not found
	ID string
}

// # Function Explanation
//
// Error returns a string describing the resource not found error for the given ID.
func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("the resource %s was not found", e.ID)
}

// # Function Explanation
//
// Is checks if the target error is an instance of ErrNotFound and returns a boolean.
func (e *ErrNotFound) Is(target error) bool {
	t, ok := target.(*ErrNotFound)
	if !ok {
		return false
	}
	return (e.ID == t.ID || t.ID == "")
}

var _ error = (*ErrInvalid)(nil)

type ErrConcurrency struct {
}

// # Function Explanation
//
// Error returns the error message for ErrConcurrency error.
func (e *ErrConcurrency) Error() string {
	return "the operation failed due to a concurrency conflict"
}

// # Function Explanation
//
// Is checks if the target error is an instance of ErrConcurrency.
func (e *ErrConcurrency) Is(target error) bool {
	_, ok := target.(*ErrConcurrency)
	return ok
}
