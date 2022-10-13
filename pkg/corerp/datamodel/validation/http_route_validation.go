// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validation

import (
	validation "github.com/project-radius/radius/pkg/armrpc/api/validation"
	datamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
)

type HTTPRouteResourceValidators struct {
}

func (v *HTTPRouteResourceValidators) ValidateRequest(*datamodel.HTTPRoute) error {
	return nil
}

func NewHTTPRouteResourceValidators() validation.Validators[datamodel.HTTPRoute] {
	return &HTTPRouteResourceValidators{}
}
