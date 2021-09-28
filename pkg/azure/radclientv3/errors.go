// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radclientv3

import (
	"fmt"
	"time"
)

// The default poll interval that we use for async radclient operations.
const PollInterval = 5 * time.Second

// RadiusError represents errors returned by Radius.
type RadiusError struct {
	Code string

	Message string
}

// Error implements the error interface for the RadiusError type.
func (e *RadiusError) Error() string {
	return fmt.Sprintf("%s\n%s", e.Code, e.Message)
}

// NewRadiusError returns a RadiusError instance for provided code and message.
func NewRadiusError(errorCode string, errorMessage string) error {
	return &RadiusError{
		Code:    errorCode,
		Message: errorMessage,
	}
}
