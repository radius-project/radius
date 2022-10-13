// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validation

import (
	validation "github.com/project-radius/radius/pkg/armrpc/api/validation"
	datamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
)

type VolumeResourceValidators struct {
}

func (v *VolumeResourceValidators) ValidateRequest(*datamodel.VolumeResource) error {
	return nil
}

func NewVolumeResourceValidators() validation.Validators[datamodel.VolumeResource] {
	return &VolumeResourceValidators{}
}
