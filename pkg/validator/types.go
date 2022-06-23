// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

type contextKey struct {
	name string
}

func (k *contextKey) String() string {
	return "validator context name: " + k.name
}
