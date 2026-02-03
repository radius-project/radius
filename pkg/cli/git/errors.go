// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

package git

import (
	"encoding/json"
	"fmt"
	"io"
)

// StructuredError represents an error with structured output support per NFR-021.
// This format enables machine-readable error handling in CI/CD pipelines.
type StructuredError struct {
	// Code is the semantic exit code (see exitcodes.go constants).
	Code int `json:"code" yaml:"code"`

	// Message is a human-readable error description.
	Message string `json:"message" yaml:"message"`

	// Details contains optional additional context about the error.
	Details string `json:"details,omitempty" yaml:"details,omitempty"`

	// Source identifies the component that generated the error.
	Source string `json:"source,omitempty" yaml:"source,omitempty"`

	// Cause contains the underlying error message if available.
	Cause string `json:"cause,omitempty" yaml:"cause,omitempty"`
}

// Error implements the error interface.
func (e *StructuredError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Details)
	}
	return e.Message
}

// ExitCode returns the semantic exit code for this error.
func (e *StructuredError) ExitCode() int {
	return e.Code
}

// Unwrap returns nil as StructuredError doesn't wrap another error directly.
// Use WithCause to attach underlying error information.
func (e *StructuredError) Unwrap() error {
	return nil
}

// WriteJSON writes the error as JSON to the given writer.
func (e *StructuredError) WriteJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(e)
}

// NewValidationError creates a StructuredError for validation failures.
func NewValidationError(message, details string) *StructuredError {
	return &StructuredError{
		Code:    ExitValidationError,
		Message: message,
		Details: details,
		Source:  "validation",
	}
}

// NewAuthError creates a StructuredError for authentication failures.
func NewAuthError(message, details string) *StructuredError {
	return &StructuredError{
		Code:    ExitAuthError,
		Message: message,
		Details: details,
		Source:  "auth",
	}
}

// NewResourceConflictError creates a StructuredError for state conflicts.
func NewResourceConflictError(message, details string) *StructuredError {
	return &StructuredError{
		Code:    ExitResourceConflict,
		Message: message,
		Details: details,
		Source:  "state",
	}
}

// NewDeploymentError creates a StructuredError for deployment failures.
func NewDeploymentError(message, details string) *StructuredError {
	return &StructuredError{
		Code:    ExitDeploymentFailure,
		Message: message,
		Details: details,
		Source:  "deploy",
	}
}

// NewGeneralError creates a StructuredError for unexpected errors.
func NewGeneralError(message string, err error) *StructuredError {
	se := &StructuredError{
		Code:    ExitGeneralError,
		Message: message,
		Source:  "general",
	}
	if err != nil {
		se.Cause = err.Error()
	}
	return se
}

// WithCause sets the underlying error cause and returns the error for chaining.
func (e *StructuredError) WithCause(err error) *StructuredError {
	if err != nil {
		e.Cause = err.Error()
	}
	return e
}

// WithSource sets the source component and returns the error for chaining.
func (e *StructuredError) WithSource(source string) *StructuredError {
	e.Source = source
	return e
}
