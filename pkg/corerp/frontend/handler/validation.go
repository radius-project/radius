// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"fmt"

	"github.com/project-radius/radius/pkg/radrp/schema"
)

type ValidatorFactory = func(resourceType string) (schema.Validator, error)

func DefaultValidatorFactory(resourceType string) (schema.Validator, error) {
	validator, ok := schema.GetValidator(resourceType)
	if !ok {
		return nil, fmt.Errorf("unsupported resource type %s", resourceType)
	}

	return validator, nil
}
