// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
)

var ErrResourceSpecifiedForManagedResource = errors.New("the 'resource' field cannot be specified when 'managed=true'")
var ErrResourceMissingForUnmanagedResource = errors.New("the 'resource' field is required when 'managed' is not specified")

func ValidateResourceID(id string, resourceType azresources.KnownType, description string) (azresources.ResourceID, error) {
	parsed, err := azresources.Parse(id)
	if err != nil {
		return azresources.ResourceID{}, errors.New("the 'resource' field must be a valid resource id")
	}

	err = parsed.ValidateResourceType(resourceType)
	if err != nil {
		return azresources.ResourceID{}, fmt.Errorf("the 'resource' field must refer to a %s", description)
	}

	return parsed, nil
}
