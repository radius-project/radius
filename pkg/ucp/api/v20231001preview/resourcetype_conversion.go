/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v20231001preview

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"

	productmanifest "github.com/radius-project/radius/deploy/manifest"
)

// ConvertTo converts from the versioned ResourceTypeResource resource to version-agnostic datamodel.
func (src *ResourceTypeResource) ConvertTo() (v1.DataModelInterface, error) {
	dst := &datamodel.ResourceType{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   to.String(src.ID),
				Name: to.String(src.Name),
				Type: datamodel.ResourceTypeResourceType,

				// NOTE: this is a child resource. It does not have a location, systemData, or tags.
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},
	}

	capabilities := []string{}
	for _, capability := range src.Properties.Capabilities {
		err := validateCapability(capability)
		if err != nil {
			return nil, err
		}

		capabilities = append(capabilities, *capability)
	}

	dst.Properties = datamodel.ResourceTypeProperties{
		Capabilities:      capabilities,
		DefaultAPIVersion: src.Properties.DefaultAPIVersion,
	}

	dst.Properties.Description = src.Properties.Description

	// The icon is written by the client as verbatim SVG bytes. The hash is
	// server-computed (read-only on the wire) so it content-addresses exactly
	// the bytes that were stored.
	dst.Properties.Icon = src.Properties.Icon
	if src.Properties.Icon != nil {
		iconBytes := []byte(*src.Properties.Icon)
		if err := datamodel.ValidateIcon(iconBytes); err != nil {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid icon: %s", err.Error()))
		}
		sum := sha256.Sum256(iconBytes)
		dst.Properties.IconHash = to.Ptr(hex.EncodeToString(sum[:]))
	} else {
		// No icon supplied — substitute the product default icon's hash so
		// every registered type has a non-nil IconHash. The bytes stay unset
		// on the record; consumers fetch them from the embedded product
		// default in-binary (deploy/manifest package). If the embedded
		// default failed to load, DefaultHash returns nil and IconHash stays
		// unset — icons are cosmetic, we degrade gracefully rather than
		// fail registration.
		dst.Properties.IconHash = productmanifest.DefaultHash()
	}

	return dst, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned ResourceTypeResource resource.
func (dst *ResourceTypeResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.ResourceType)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = new(dm.ID)
	dst.Name = new(dm.Name)
	dst.Type = new(dm.Type)

	// NOTE: this is a child resource. It does not have a location, systemData, or tags.

	dst.Properties = &ResourceTypeProperties{
		ProvisioningState: new(ProvisioningState(dm.InternalMetadata.AsyncProvisioningState)),
		Capabilities:      to.SliceOfPtrs(dm.Properties.Capabilities...),
		DefaultAPIVersion: dm.Properties.DefaultAPIVersion,
		Description:       dm.Properties.Description,
		Icon:              dm.Properties.Icon,
		IconHash:          dm.Properties.IconHash,
	}

	return nil
}

func validateCapability(input *string) error {
	if input == nil {
		return v1.NewClientErrInvalidRequest("capability cannot be null")
	}

	if *input == datamodel.CapabilityManualResourceProvisioning {
		return nil
	}

	return v1.NewClientErrInvalidRequest(fmt.Sprintf("capability %q is not recognized. Supported capabilities: %s", *input, datamodel.CapabilityManualResourceProvisioning))
}
