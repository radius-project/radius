// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"fmt"

	"github.com/Azure/radius/pkg/radrp/schemav3"
)

type ValidatorFactory = func(resourceType string) (schemav3.Validator, error)

func DefaultValidatorFactory(resourceType string) (schemav3.Validator, error) {
	validator, ok := schemav3.GetValidator(resourceType)
	if !ok {
		return nil, fmt.Errorf("unsupported resource type %s", resourceType)
	}

	return validator, nil
}
