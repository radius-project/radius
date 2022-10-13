// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validation

import (
	validation "github.com/project-radius/radius/pkg/armrpc/api/validation"
	datamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
)

type ApplicationResourceValidators struct {
}

func (v *ApplicationResourceValidators) ValidateRequest(*datamodel.Application) error {
	return nil
}

func NewApplicationResourceValidators() validation.Validators[datamodel.Application] {
	return &ApplicationResourceValidators{}
}
