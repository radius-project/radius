// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220901privatepreview

import (
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

const (
	EnvironmentComputeKindKubernetes = "kubernetes"
)

// ConvertTo converts from the versioned Environment resource to version-agnostic datamodel.
func (src *PlaneResource) ConvertTo() (conv.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.

	if src.Properties.Kind == nil || *src.Properties.Kind == "" {
		return nil, &conv.ErrModelConversion{PropertyName: "$.properties.kind", ValidValue: "63 characters or less"}
	}

	if *src.Properties.Kind == PlaneKindUCPNative && (src.Properties.ResourceProviders == nil || len(src.Properties.ResourceProviders) == 0) {
		return nil, &conv.ErrModelConversion{PropertyName: "$.properties.resourceProviders", ValidValue: "at least one provided"}
	} else if *src.Properties.Kind == PlaneKindAzure && (src.Properties.URL == nil || *src.Properties.URL == "") {
		return nil, &conv.ErrModelConversion{PropertyName: "$.properties.URL", ValidValue: "non-empty string"}
	}

	converted := &datamodel.Plane{
		TrackedResource: v1.TrackedResource{
			ID:   to.String(src.ID),
			Name: to.String(src.Name),
			Type: to.String(src.Type),
		},
		Properties: datamodel.PlaneProperties{
			Kind:              datamodel.PlaneKind(*src.Properties.Kind),
			URL:               src.Properties.URL,
			ResourceProviders: src.Properties.ResourceProviders,
		},
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Plane resource.
func (dst *PlaneResource) ConvertFrom(src conv.DataModelInterface) error {
	// TODO: Improve the validation.
	plane, ok := src.(*datamodel.Plane)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = &plane.TrackedResource.ID
	dst.Name = &plane.TrackedResource.Name
	dst.Type = &plane.TrackedResource.Type

	dst.Properties = &PlaneResourceProperties{
		Kind:              (*PlaneKind)(&plane.Properties.Kind),
		URL:               plane.Properties.URL,
		ResourceProviders: plane.Properties.ResourceProviders,
	}

	return nil
}
