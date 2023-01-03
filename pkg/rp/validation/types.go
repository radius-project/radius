// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// validation contains functionality for validating user input and reporting error messages.
package validation

import "strings"

type Validator struct {
	messages []string
}

func (v *Validator) HasErrors() bool {
	return len(v.messages) > 0
}

func (v *Validator) FormatError() string {
	if len(v.messages) == 0 {
		return "" // Not an error
	}

	if len(v.messages) == 1 {
		return v.messages[0]
	}

	return "validation found multiple errors:\n\n" + strings.Join(v.messages, "\n")
}

func Optional() FieldOptionOptional {
	return FieldOptionOptional{}
}

type FieldOptionOptional struct {
}

func (o FieldOptionOptional) Apply(options *FieldOptions) {
	options.Optional = true
}

func (o FieldOptionOptional) private() {}

type FieldOptions struct {
	Optional bool
}

type Option interface {
	Apply(options *FieldOptions)

	private() // Prevent other packages from implementing Option
}
