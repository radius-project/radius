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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ValidationError
		expected string
	}{
		{
			name: "error with field",
			err: &ValidationError{
				Type:    ErrorTypeSchema,
				Field:   "properties.name",
				Message: "field is required",
			},
			expected: "SchemaError error at 'properties.name': field is required",
		},
		{
			name: "error without field",
			err: &ValidationError{
				Type:    ErrorTypeConstraint,
				Field:   "",
				Message: "allOf is not supported",
			},
			expected: "ConstraintError error: allOf is not supported",
		},
		{
			name: "format error",
			err: &ValidationError{
				Type:    ErrorTypeFormat,
				Field:   "timestamp",
				Message: "invalid date format",
			},
			expected: "FormatError error at 'timestamp': invalid date format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestNewSchemaError(t *testing.T) {
	err := NewSchemaError("field.name", "test message")
	
	require.Equal(t, ErrorTypeSchema, err.Type)
	require.Equal(t, "field.name", err.Field)
	require.Equal(t, "test message", err.Message)
	require.Equal(t, "SchemaError error at 'field.name': test message", err.Error())
}

func TestNewConstraintError(t *testing.T) {
	err := NewConstraintError("constraint.field", "constraint violated")
	
	require.Equal(t, ErrorTypeConstraint, err.Type)
	require.Equal(t, "constraint.field", err.Field)
	require.Equal(t, "constraint violated", err.Message)
	require.Equal(t, "ConstraintError error at 'constraint.field': constraint violated", err.Error())
}

func TestNewFormatError(t *testing.T) {
	err := NewFormatError("date.field", "date", "invalid format")
	
	require.Equal(t, ErrorTypeFormat, err.Type)
	require.Equal(t, "date.field", err.Field)
	require.Equal(t, "invalid format", err.Message)
	require.Equal(t, "FormatError error at 'date.field': invalid format", err.Error())
}

func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name     string
		errors   *ValidationErrors
		expected string
	}{
		{
			name:     "no errors",
			errors:   &ValidationErrors{},
			expected: "validation failed",
		},
		{
			name: "single error",
			errors: &ValidationErrors{
				Errors: []*ValidationError{
					NewSchemaError("field1", "error message"),
				},
			},
			expected: "SchemaError error at 'field1': error message",
		},
		{
			name: "multiple errors",
			errors: &ValidationErrors{
				Errors: []*ValidationError{
					NewSchemaError("field1", "first error"),
					NewConstraintError("field2", "second error"),
				},
			},
			expected: "validation failed with 2 errors:\n  1. SchemaError error at 'field1': first error\n  2. ConstraintError error at 'field2': second error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.errors.Error()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestValidationErrors_Add(t *testing.T) {
	errors := &ValidationErrors{}
	
	// Test adding valid error
	err1 := NewSchemaError("field1", "message1")
	errors.Add(err1)
	require.Len(t, errors.Errors, 1)
	require.Equal(t, err1, errors.Errors[0])
	
	// Test adding another error
	err2 := NewConstraintError("field2", "message2")
	errors.Add(err2)
	require.Len(t, errors.Errors, 2)
	require.Equal(t, err2, errors.Errors[1])
	
	// Test adding nil error (should be ignored)
	errors.Add(nil)
	require.Len(t, errors.Errors, 2)
}

func TestValidationErrors_HasErrors(t *testing.T) {
	errors := &ValidationErrors{}
	
	// Initially no errors
	require.False(t, errors.HasErrors())
	
	// Add an error
	errors.Add(NewSchemaError("field", "message"))
	require.True(t, errors.HasErrors())
	
	// Test with multiple errors
	errors.Add(NewConstraintError("field2", "message2"))
	require.True(t, errors.HasErrors())
}

func TestErrorType_Constants(t *testing.T) {
	require.Equal(t, ErrorType("SchemaError"), ErrorTypeSchema)
	require.Equal(t, ErrorType("ConstraintError"), ErrorTypeConstraint)
	require.Equal(t, ErrorType("FormatError"), ErrorTypeFormat)
}