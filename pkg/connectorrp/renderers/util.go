// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"fmt"

	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func (r *ErrClientRenderer) Error() string {
	return fmt.Sprintf("code %v: err %v", r.Code, r.Message)
}

func NewClientErrInvalidRequest(message string) *ErrClientRenderer {
	err := new(ErrClientRenderer)
	err.Message = message
	err.Code = armerrors.Invalid
	return err
}

func ValidateApplicationID(application string) error {
	if application != "" {
		_, err := resources.Parse(application)
		if err != nil {
			return NewClientErrInvalidRequest(fmt.Sprintf("failed to parse application from the property: %s ", err.Error()))
		}
	}
	return nil
}
