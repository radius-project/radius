// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Container resource to version-agnostic datamodel.
func (src *ContainerResource) ConvertTo() (api.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.

	connections := make(map[string]datamodel.ConnectionProperties)
	for key, val := range src.Properties.Connections {
		roles := []string{}
		var kind datamodel.Kind
		if val != nil {
			if val.Iam != nil {
				for _, r := range val.Iam.Roles {
					roles = append(roles, to.String(r))
				}

				kind = toKindDataModel(val.Iam.Kind)
			}

			connections[key] = datamodel.ConnectionProperties{
				Source:                to.String(val.Source),
				DisableDefaultEnvVars: to.Bool(val.DisableDefaultEnvVars),
				Iam: datamodel.IamProperties{
					Kind:  kind,
					Roles: roles,
				},
			}
		}
	}
	/*
		probe := ConvertToHealthProbePropertiesClassification(src.Properties.Container.LivenessProbe)

		converted_probe := datamodel.HealthProbeProperties{
			Kind:                to.String(probe.Kind),
			FailureThreshold:    to.Float32(probe.FailureThreshold),
			InitialDelaySeconds: to.Float32(probe.InitialDelaySeconds),
			PeriodSeconds:       to.Float32(probe.PeriodSeconds),
		}
	*/

	converted := &datamodel.ContainerResource{
		TrackedResource: basedatamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.ContainerProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Application:       to.String(src.Properties.Application),
			Connections:       connections,
			Container: datamodel.Container{
				Image: to.String(src.Properties.Container.Image),
				Env:   to.StringMap(src.Properties.Container.Env),
			},
		},
		InternalMetadata: basedatamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Container resource.
func (dst *ContainerResource) ConvertFrom(src api.DataModelInterface) error {
	c, ok := src.(*datamodel.ContainerResource)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(c.ID)
	dst.Name = to.StringPtr(c.Name)
	dst.Type = to.StringPtr(c.Type)
	dst.SystemData = fromSystemDataModel(c.SystemData)
	dst.Location = to.StringPtr(c.Location)
	dst.Tags = *to.StringMapPtr(c.Tags)
	dst.Properties = &ContainerProperties{
		ProvisioningState: fromProvisioningStateDataModel(c.Properties.ProvisioningState),
		Application:       to.StringPtr(c.Properties.Application),
	}

	return nil
}

/*
func ConvertToHealthProbePropertiesClassification(i interface{}) datamodel.HealthProbePropertiesClassification {
	var p datamodel.HealthProbePropertiesClassification
	switch i.(type) {
	case ExecHealthProbeProperties:
	case HTTPGetHealthProbeProperties:
	case TCPHealthProbeProperties:
	default:
	}
	return p
}
*/

func toKindDataModel(kind *Kind) datamodel.Kind {
	if kind == nil {
		return datamodel.KindAzure // TODO: need to define default kind
	}

	switch *kind {
	case KindAzure:
		return datamodel.KindAzure
	default:
		return datamodel.KindAzure
	}
}
