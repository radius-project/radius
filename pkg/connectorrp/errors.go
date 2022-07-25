// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package conv

import (
	"fmt"

	"github.com/project-radius/radius/pkg/radrp/armerrors"
)

type ErrClientConnector struct {
	Code    string
	Message string
}

func (r *ErrClientConnector) Error() string {
	return fmt.Sprintf("code %v: err %v", r.Code, r.Message)
}

func NewClientErrInvalidRequest(message string) *ErrClientConnector {
	err := new(ErrClientConnector)
	err.Message = message
	err.Code = armerrors.Invalid
	return err
}
