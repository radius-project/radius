// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/corerp/api"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Environment resource to version-agnostic datamodel.
func (src *EnvironmentResource) ConvertTo() (api.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	// TODO: Improve the validation.
	converted := &datamodel.Environment{
		TrackedResource: datamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.EnvironmentProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Compute: datamodel.EnvironmentCompute{
				Kind:       toEnvironmentComputeKindDataModel(src.Properties.Compute.Kind),
				ResourceID: to.String(src.Properties.Compute.ResourceID),
			},
		},
		InternalMetadata: datamodel.InternalMetadata{
			APIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment resource.
func (dst *EnvironmentResource) ConvertFrom(src api.DataModelInterface) error {
	// TODO: Improve the validation.
	env, ok := src.(*datamodel.Environment)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(env.ID)
	dst.Name = to.StringPtr(env.Name)
	dst.Type = to.StringPtr(env.Type)
	dst.SystemData = fromSystemDataModel(env.SystemData)
	dst.Location = to.StringPtr(env.Location)
	dst.Tags = *to.StringMapPtr(env.Tags)
	dst.Properties = &EnvironmentProperties{
		ProvisioningState: fromProvisioningStateDataModel(env.Properties.ProvisioningState),
		Compute: &EnvironmentCompute{
			Kind:       fromEnvironmentComputeKind(env.Properties.Compute.Kind),
			ResourceID: to.StringPtr(env.Properties.Compute.ResourceID),
		},
	}

	return nil
}

func toEnvironmentComputeKindDataModel(kind *EnvironmentComputeKind) datamodel.EnvironmentComputeKind {
	switch *kind {
	case EnvironmentComputeKindKubernetes:
		return datamodel.KubernetesComputeKind
	default:
		return datamodel.UnknownComputeKind
	}
}

func fromEnvironmentComputeKind(kind datamodel.EnvironmentComputeKind) *EnvironmentComputeKind {
	var k EnvironmentComputeKind
	switch kind {
	case datamodel.KubernetesComputeKind:
		k = EnvironmentComputeKindKubernetes
	default:
		k = EnvironmentComputeKindKubernetes // 2022-03-15-privatprevie supports only kubernetes.
	}

	return &k
}
