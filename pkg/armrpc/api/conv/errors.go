// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package conv

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidModelConversion is the error when converting model is invalid.
	ErrInvalidModelConversion = errors.New("invalid model conversion")
)

// ErrModelConversion represents an invalid property error.
type ErrModelConversion struct {
	PropertyName string
	ValidValue   string
}

func (e *ErrModelConversion) Error() string {
	return fmt.Sprintf("%s must be %s.", e.PropertyName, e.ValidValue)
}

func (e *ErrModelConversion) Is(target error) bool {
	t, ok := target.(*ErrModelConversion)
	if !ok {
		return false
	}

	return (e.PropertyName == t.PropertyName && e.ValidValue == t.ValidValue)
}
