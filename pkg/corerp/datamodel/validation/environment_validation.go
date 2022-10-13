// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validation

import (
	validation "github.com/project-radius/radius/pkg/armrpc/api/validation"
	datamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
)

type EnvironmentResourceValidators struct {
}

func (v *EnvironmentResourceValidators) ValidateRequest(*datamodel.Environment) error {
	return nil
}

func NewEnvironmentResourceValidators() validation.Validators[datamodel.Environment] {
	return &EnvironmentResourceValidators{}
}
