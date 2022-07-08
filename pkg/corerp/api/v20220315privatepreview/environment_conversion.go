// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

const (
	EnvironmentComputeKindKubernetes = "kubernetes"
)

// ConvertTo converts from the versioned Environment resource to version-agnostic datamodel.
func (src *EnvironmentResource) ConvertTo() (conv.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	// TODO: Improve the validation.
	converted := &datamodel.Environment{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.EnvironmentProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
		},
		InternalMetadata: v1.InternalMetadata{
			CreatedAPIVersion: Version,
			UpdatedAPIVersion: Version,
		},
	}

	envCompute, err := toEnvironmentComputeDataModel(src.Properties.Compute)
	if err != nil {
		return nil, err
	}

	converted.Properties.Compute = *envCompute

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment resource.
func (dst *EnvironmentResource) ConvertFrom(src conv.DataModelInterface) error {
	// TODO: Improve the validation.
	env, ok := src.(*datamodel.Environment)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(env.ID)
	dst.Name = to.StringPtr(env.Name)
	dst.Type = to.StringPtr(env.Type)
	dst.SystemData = fromSystemDataModel(env.SystemData)
	dst.Location = to.StringPtr(env.Location)
	dst.Tags = *to.StringMapPtr(env.Tags)
	dst.Properties = &EnvironmentProperties{
		ProvisioningState: fromProvisioningStateDataModel(env.Properties.ProvisioningState),
	}

	dst.Properties.Compute = fromEnvironmentComputeDataModel(&env.Properties.Compute)
	if dst.Properties.Compute == nil {
		return conv.ErrInvalidModelConversion
	}

	return nil
}

func toEnvironmentComputeDataModel(h EnvironmentComputeClassification) (*datamodel.EnvironmentCompute, error) {
	switch v := h.(type) {
	case *KubernetesCompute:
		k, err := toEnvironmentComputeKindDataModel(*v.Kind)
		if err != nil {
			return nil, err
		}

		if v.Namespace == nil || len(*v.Namespace) == 0 || len(*v.Namespace) >= 64 {
			return nil, &conv.ErrModelConversion{PropertyName: "$.properties.compute.namespace", ValidValue: "63 characters or less"}
		}

		return &datamodel.EnvironmentCompute{
			Kind: k,
			KubernetesCompute: datamodel.KubernetesComputeProperties{
				ResourceID: *v.ResourceID,
				Namespace:  *v.Namespace,
			},
		}, nil
	default:
		return nil, conv.ErrInvalidModelConversion
	}
}

func fromEnvironmentComputeDataModel(envCompute *datamodel.EnvironmentCompute) EnvironmentComputeClassification {
	switch envCompute.Kind {
	case datamodel.KubernetesComputeKind:
		return &KubernetesCompute{
			EnvironmentCompute: EnvironmentCompute{
				Kind:       fromEnvironmentComputeKind(envCompute.Kind),
				ResourceID: to.StringPtr(envCompute.KubernetesCompute.ResourceID),
			},
			Namespace: &envCompute.KubernetesCompute.Namespace,
		}
	default:
		return nil
	}
}

func toEnvironmentComputeKindDataModel(kind string) (datamodel.EnvironmentComputeKind, error) {
	switch kind {
	case EnvironmentComputeKindKubernetes:
		return datamodel.KubernetesComputeKind, nil
	default:
		return datamodel.UnknownComputeKind, &conv.ErrModelConversion{PropertyName: "$.properties.compute.kind", ValidValue: "[kubernetes]"}
	}
}

func fromEnvironmentComputeKind(kind datamodel.EnvironmentComputeKind) *string {
	var k string
	switch kind {
	case datamodel.KubernetesComputeKind:
		k = EnvironmentComputeKindKubernetes
	default:
		k = EnvironmentComputeKindKubernetes // 2022-03-15-privatprevie supports only kubernetes.
	}

	return &k
}
