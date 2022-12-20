// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package conv

import (
	"errors"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
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

func NewClientErrInvalidRequest(message string) *ErrClientRP {
	err := new(ErrClientRP)
	err.Message = message
	err.Code = v1.CodeInvalid
	return err
}
