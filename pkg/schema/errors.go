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

package schema

import (
	"fmt"
	"strings"
)

// ErrorType represents the type of validation error
type ErrorType string

const (
	// ErrorTypeSchema indicates a schema structure validation error
	ErrorTypeSchema ErrorType = "SchemaError"

	// ErrorTypeConstraint indicates a Radius constraint violation
	ErrorTypeConstraint ErrorType = "ConstraintError"

	// ErrorTypeFormat indicates a format validation error
	ErrorTypeFormat ErrorType = "FormatError"
)

// ValidationError represents a schema validation error
type ValidationError struct {
	Type    ErrorType
	Field   string
	Message string
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s error at '%s': %s", e.Type, e.Field, e.Message)
	}
	return fmt.Sprintf("%s error: %s", e.Type, e.Message)
}

// ValidationErrors represents a collection of validation errors
type ValidationErrors struct {
	Errors []*ValidationError
}

// NewSchemaError creates a schema validation error
func NewSchemaError(field, message string) *ValidationError {
	return &ValidationError{
		Type:    ErrorTypeSchema,
		Field:   field,
		Message: message,
	}
}

// NewConstraintError creates a Radius constraint validation error
func NewConstraintError(field, message string) *ValidationError {
	return &ValidationError{
		Type:    ErrorTypeConstraint,
		Field:   field,
		Message: message,
	}
}

// NewFormatError creates a new format validation error
func NewFormatError(field, format, message string) *ValidationError {
	return &ValidationError{
		Type:    ErrorTypeFormat,
		Field:   field,
		Message: message,
	}
}

// Error implements the error interface
func (ve *ValidationErrors) Error() string {
	switch len(ve.Errors) {
	case 0:
		return "validation failed"
	case 1:
		return ve.Errors[0].Error()
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "validation failed with %d errors:\n", len(ve.Errors))
		for i, err := range ve.Errors {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, err.Error())
		}
		return strings.TrimSuffix(b.String(), "\n")
	}
}

// Add adds a validation error to the collection
func (ve *ValidationErrors) Add(err *ValidationError) {
	if err != nil {
		ve.Errors = append(ve.Errors, err)
	}
}

// HasErrors returns true if there are any validation errors
func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}
