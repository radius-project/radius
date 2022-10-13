// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validation

// Validators interface is a collection of resource validation functions.
type Validators[T any] interface {
	ValidateRequest(model *T) error
}
