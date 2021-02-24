// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package curp

import (
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/curp/armerrors"
	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/go-playground/validator/v10"
)

// ErrUnsupportedWorkload indicates an unsupported workload type.
var ErrUnsupportedWorkload = errors.New("unsupported workload type")

// StatusCodeError represents an error with a known mapping to an HTTP status code.
type StatusCodeError interface {
	error
	StatusCode() int
	ErrorResponse() armerrors.ErrorResponse
}

// BadRequestError represents a generic bad request.
type BadRequestError struct {
	Message string
}

func (e BadRequestError) Error() string {
	return e.Message
}

// StatusCode implmements StatusCodeError for BadRequestError.
func (e BadRequestError) StatusCode() int {
	return 400
}

// ErrorResponse implements ErrorResponse for BadRequestError.
func (e BadRequestError) ErrorResponse() armerrors.ErrorResponse {
	return armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Message: e.Error(),
		},
	}
}

// NotFoundError represents an entity not being found.
type NotFoundError struct {
	ID resources.ResourceID
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("the resource with id '%v' was not found", e.ID)
}

// StatusCode implmements StatusCodeError for NotFoundError
func (e NotFoundError) StatusCode() int {
	return 404
}

// ErrorResponse implements ErrorResponse for NotFoundError.
func (e NotFoundError) ErrorResponse() armerrors.ErrorResponse {
	return armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Target:  e.ID.ID,
			Message: e.Error(),
		},
	}
}

// ConflictError represents an action that cannot be taken due to concurrency or a logical conflict.
type ConflictError struct {
	Message string
}

func (e ConflictError) Error() string {
	return e.Message
}

// StatusCode implmements StatusCodeError for ConflictError
func (e ConflictError) StatusCode() int {
	return 409
}

// ErrorResponse implements ErrorResponse for ConflictError.
func (e ConflictError) ErrorResponse() armerrors.ErrorResponse {
	return armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Message: e.Error(),
		},
	}
}

// ValidationError indicates a validation error.
type ValidationError struct {
	Value  interface{}
	Errors validator.ValidationErrors
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("the value of %T had validation errors: %s", e.Value, e.Errors)
}

// StatusCode implmements StatusCodeError for ValidationError
func (e ValidationError) StatusCode() int {
	return 400
}

// ErrorResponse implements ErrorResponse for ValidationError.
func (e ValidationError) ErrorResponse() armerrors.ErrorResponse {
	res := armerrors.ErrorResponse{
		Error: armerrors.ErrorDetails{
			Message: e.Error(),
		},
	}

	for _, fe := range e.Errors {
		if err, ok := fe.(error); ok {
			detail := armerrors.ErrorDetails{
				Target:  fe.Field(),
				Message: err.Error(),
			}
			res.Error.Details = append(res.Error.Details, detail)
		}
	}

	return res
}
