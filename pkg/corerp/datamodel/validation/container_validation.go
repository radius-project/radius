// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validation

import (
	validation "github.com/project-radius/radius/pkg/armrpc/api/validation"
	datamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
)

type ContainerResourceValidators struct {
}

func (v *ContainerResourceValidators) ValidateRequest(*datamodel.ContainerResource) error {
	return nil
}

func NewContainerResourceValidators() validation.Validators[datamodel.ContainerResource] {
	return &ContainerResourceValidators{}
}
