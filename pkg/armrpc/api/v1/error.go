// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

func (e *ErrModelConversion) Error() string {
	return fmt.Sprintf("%s must be %s.", e.PropertyName, e.ValidValue)
}

func (e *ErrModelConversion) Is(target error) bool {
	_, ok := target.(*ErrModelConversion)
	return ok
}

type ErrClientRP struct {
	Code    string
	Message string
}

func (r *ErrClientRP) Error() string {
	return fmt.Sprintf("code %v: err %v", r.Code, r.Message)
}
func (e *ErrClientRP) Is(target error) bool {
	_, ok := target.(*ErrClientRP)
	return ok
}

func NewClientErrInvalidRequest(message string) *ErrClientRP {
	err := new(ErrClientRP)
	err.Message = message
	err.Code = CodeInvalid
	return err
}
