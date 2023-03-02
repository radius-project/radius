// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

var _ error = (*ErrInvalid)(nil)

type ErrInvalid struct {
	Message string
}

func (e *ErrInvalid) Error() string {
	return e.Message
}

func (e *ErrInvalid) Is(target error) bool {
	t, ok := target.(*ErrInvalid)
	if !ok {
		return false
	}

	return (e.Message == t.Message || t.Message == "")
}

type ErrNotFound struct {
}

func (e *ErrNotFound) Error() string {
	return "ucp/store - the resource was not found"
}

func (e *ErrNotFound) Is(target error) bool {
	_, ok := target.(*ErrNotFound)
	return ok
}

var _ error = (*ErrInvalid)(nil)

type ErrConcurrency struct {
}

func (e *ErrConcurrency) Error() string {
	return "the operation failed due to a concurrency conflict"
}

func (e *ErrConcurrency) Is(target error) bool {
	_, ok := target.(*ErrConcurrency)
	return ok
}
