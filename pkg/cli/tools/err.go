// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tools

var _ error = (ErrToolNotFound)(ErrToolNotFound{})

type ErrToolNotFound struct {
	Tool    string
	Message string
}

func (e ErrToolNotFound) Error() string {
	return e.Message
}
