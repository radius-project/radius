// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloads

import (
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/radrp/resources"
)

var ErrResourceSpecifiedForManagedResource = errors.New("the 'resource' field cannot be specified when 'managed=true'")
var ErrResourceMissingForUnmanagedResource = errors.New("the 'resource' field is required when 'managed' is not specified")

func ValidateResourceID(id string, resourceType azresources.KnownType, description string) (resources.ResourceID, error) {
	parsed, err := azresources.Parse(id)
	if err != nil {
		return resources.ResourceID{}, errors.New("the 'resource' field must be a valid resource id.")
	}

	err = parsed.ValidateResourceType(resourceType)
	if err != nil {
		return resources.ResourceID{}, fmt.Errorf("the 'resource' field must refer to a %s", description)
	}

	return resources.ResourceID{ResourceID: parsed}, err
}
