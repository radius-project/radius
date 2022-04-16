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
	Message string
}

func (e *ErrNotFound) Error() string {
	return e.Message
}

func (e *ErrNotFound) Is(target error) bool {
	t, ok := target.(*ErrNotFound)
	if !ok {
		return false
	}

	return (e.Message == t.Message || t.Message == "")
}
