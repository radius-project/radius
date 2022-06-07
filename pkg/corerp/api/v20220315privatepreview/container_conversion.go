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

	livenessProbe := toHealthProbePropertiesClassificationDataModel(src.Properties.Container.LivenessProbe)

	readinessProbe := toHealthProbePropertiesClassificationDataModel(src.Properties.Container.ReadinessProbe)

	ports := make(map[string]datamodel.ContainerPort)
	for key, val := range src.Properties.Container.Ports {
		ports[key] = datamodel.ContainerPort{
			ContainerPort: to.Int32(val.ContainerPort),
			Protocol:      toProtocolDataModel(val.Protocol),
			Provides:      to.String(val.Provides),
		}
	}

	volumes := make(map[string]datamodel.VolumeClassification)
	for key, val := range src.Properties.Container.Volumes {
		volumes[key] = toVolumeClassificationDataModel(val)
	}

	extensions := []datamodel.ExtensionClassification{}
	for _, e := range src.Properties.Extensions {
		extensions = append(extensions, toExtensionClassificationDataModel(e))
	}

	resourceStatus := basedatamodel.ResourceStatus{}
	if src.Properties.BasicResourceProperties.Status != nil {
		resourceStatus = basedatamodel.ResourceStatus{
			OutputResources: src.Properties.BasicResourceProperties.Status.OutputResources,
		}
	}

	converted := &datamodel.ContainerResource{
		TrackedResource: basedatamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.ContainerProperties{
			BasicResourceProperties: basedatamodel.BasicResourceProperties{
				Status: resourceStatus,
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Application:       to.String(src.Properties.Application),
			Connections:       connections,
			Container: datamodel.Container{
				Image:          to.String(src.Properties.Container.Image),
				Env:            to.StringMap(src.Properties.Container.Env),
				LivenessProbe:  livenessProbe,
				Ports:          ports,
				ReadinessProbe: readinessProbe,
				Volumes:        volumes,
			},
			Extensions: extensions,
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

	connections := make(map[string]*ConnectionProperties)
	for key, val := range c.Properties.Connections {
		roles := []*string{}
		var kind *Kind

		for _, r := range val.Iam.Roles {
			roles = append(roles, to.StringPtr(r))
		}

		kind = fromKindDataModel(val.Iam.Kind)

		connections[key] = &ConnectionProperties{
			Source:                to.StringPtr(val.Source),
			DisableDefaultEnvVars: to.BoolPtr(val.DisableDefaultEnvVars),
			Iam: &IamProperties{
				Kind:  kind,
				Roles: roles,
			},
		}
	}

	livenessProbe := fromHealthProbePropertiesClassificationDataModel(c.Properties.Container.LivenessProbe)

	readinessProbe := fromHealthProbePropertiesClassificationDataModel(c.Properties.Container.ReadinessProbe)

	ports := make(map[string]*ContainerPort)
	for key, val := range c.Properties.Container.Ports {
		ports[key] = &ContainerPort{
			ContainerPort: to.Int32Ptr(val.ContainerPort),
			Protocol:      fromProtocolDataModel(val.Protocol),
			Provides:      to.StringPtr(val.Provides),
		}
	}

	volumes := make(map[string]VolumeClassification)
	for key, val := range c.Properties.Container.Volumes {
		volumes[key] = fromVolumeClassificationDataModel(val)
	}

	extensions := []ExtensionClassification{}
	for _, e := range c.Properties.Extensions {
		extensions = append(extensions, fromExtensionClassificationDataModel(e))
	}

	dst.ID = to.StringPtr(c.ID)
	dst.Name = to.StringPtr(c.Name)
	dst.Type = to.StringPtr(c.Type)
	dst.SystemData = fromSystemDataModel(c.SystemData)
	dst.Location = to.StringPtr(c.Location)
	dst.Tags = *to.StringMapPtr(c.Tags)
	dst.Properties = &ContainerProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: c.Properties.BasicResourceProperties.Status.OutputResources,
			},
		},
		ProvisioningState: fromProvisioningStateDataModel(c.Properties.ProvisioningState),
		Application:       to.StringPtr(c.Properties.Application),
		Connections:       connections,
		Container: &Container{
			Image:          to.StringPtr(c.Properties.Container.Image),
			Env:            *to.StringMapPtr(c.Properties.Container.Env),
			LivenessProbe:  livenessProbe,
			Ports:          ports,
			ReadinessProbe: readinessProbe,
			Volumes:        volumes,
		},
		Extensions: extensions,
	}

	return nil
}

func toHealthProbePropertiesClassificationDataModel(h HealthProbePropertiesClassification) datamodel.HealthProbePropertiesClassification {
	switch c := h.(type) {
	case *ExecHealthProbeProperties:
		converted := &datamodel.ExecHealthProbeProperties{
			HealthProbeProperties: toHealthProbeDataModelProperties(c.HealthProbeProperties),
			Command:               to.String(c.Command),
		}
		return converted
	case *HealthProbeProperties:
		converted := toHealthProbeDataModelProperties(*c)
		return &converted
	case *HTTPGetHealthProbeProperties:
		converted := &datamodel.HTTPGetHealthProbeProperties{
			HealthProbeProperties: toHealthProbeDataModelProperties(c.HealthProbeProperties),
			ContainerPort:         to.Int32(c.ContainerPort),
			Path:                  to.String(c.Path),
			Headers:               to.StringMap(c.Headers),
		}
		return converted

	case *TCPHealthProbeProperties:
		converted := &datamodel.TCPHealthProbeProperties{
			HealthProbeProperties: toHealthProbeDataModelProperties(c.HealthProbeProperties),
			ContainerPort:         to.Int32(c.ContainerPort),
		}
		return converted
	}
	return nil
}

func fromHealthProbePropertiesClassificationDataModel(h datamodel.HealthProbePropertiesClassification) HealthProbePropertiesClassification {
	switch c := h.(type) {
	case *datamodel.ExecHealthProbeProperties:
		converted := ExecHealthProbeProperties{
			HealthProbeProperties: fromHealthProbeDataModelProperties(c.HealthProbeProperties),
			Command:               to.StringPtr(c.Command),
		}
		return &converted
	case *datamodel.HealthProbeProperties:
		converted := fromHealthProbeDataModelProperties(*c)
		return &converted
	case *datamodel.HTTPGetHealthProbeProperties:
		converted := HTTPGetHealthProbeProperties{
			HealthProbeProperties: fromHealthProbeDataModelProperties(c.HealthProbeProperties),
			ContainerPort:         to.Int32Ptr(c.ContainerPort),
			Path:                  to.StringPtr(c.Path),
			Headers:               *to.StringMapPtr(c.Headers),
		}
		return &converted

	case *datamodel.TCPHealthProbeProperties:
		converted := TCPHealthProbeProperties{
			HealthProbeProperties: fromHealthProbeDataModelProperties(c.HealthProbeProperties),
			ContainerPort:         to.Int32Ptr(c.ContainerPort),
		}
		return &converted
	}
	return nil
}

func toKindDataModel(kind *Kind) datamodel.Kind {
	switch *kind {
	case KindAzure:
		return datamodel.KindAzure
	default:
		return datamodel.KindAzure
	}
}

func fromKindDataModel(kind datamodel.Kind) *Kind {
	var k Kind
	switch kind {
	case datamodel.KindAzure:
		k = KindAzure
	default:
		k = KindAzure
	}
	return &k
}

func toProtocolDataModel(protocol *Protocol) datamodel.Protocol {
	switch *protocol {
	case ProtocolHTTP:
		return datamodel.ProtocolHTTP
	case ProtocolGrpc:
		return datamodel.ProtocolGrpc
	case ProtocolTCP:
		return datamodel.ProtocolTCP
	case ProtocolUDP:
		return datamodel.ProtocolUDP
	default:
		return datamodel.ProtocolHTTP
	}
}

func fromProtocolDataModel(protocol datamodel.Protocol) *Protocol {
	var p Protocol
	switch protocol {
	case datamodel.ProtocolHTTP:
		p = ProtocolHTTP
	case datamodel.ProtocolGrpc:
		p = ProtocolGrpc
	case datamodel.ProtocolTCP:
		p = ProtocolTCP
	case datamodel.ProtocolUDP:
		p = ProtocolUDP
	default:
		p = ProtocolHTTP
	}
	return &p
}

func toVolumeClassificationDataModel(h VolumeClassification) datamodel.VolumeClassification {
	switch c := h.(type) {
	case *EphemeralVolume:
		converted := datamodel.EphemeralVolume{
			Volume:       toVolumeDataModel(c.Volume),
			ManagedStore: toManagedStoreDataModel(c.ManagedStore),
		}
		return converted
	case *Volume:
		converted := toVolumeDataModel(*c)
		return converted
	case *PersistentVolume:
		converted := datamodel.PersistentVolume{
			Volume: toVolumeDataModel(c.Volume),
			Source: to.String(c.Source),
			Rbac:   toRbacDataModel(c.Rbac),
		}
		return converted
	}
	return nil
}

func fromVolumeClassificationDataModel(h datamodel.VolumeClassification) VolumeClassification {
	switch c := h.(type) {
	case datamodel.EphemeralVolume:
		converted := EphemeralVolume{
			Volume:       fromVolumeDataModel(c.Volume),
			ManagedStore: fromManagedStoreDataModel(c.ManagedStore),
		}
		return converted.GetVolume()
	case datamodel.Volume:
		converted := fromVolumeDataModel(c)
		return converted.GetVolume()
	case datamodel.PersistentVolume:
		converted := PersistentVolume{
			Volume: fromVolumeDataModel(c.Volume),
			Source: to.StringPtr(c.Source),
			Rbac:   fromRbacDataModel(c.Rbac),
		}
		return converted.GetVolume()
	}
	return nil
}

func toManagedStoreDataModel(managedStore *ManagedStore) datamodel.ManagedStore {
	switch *managedStore {
	case ManagedStoreDisk:
		return datamodel.ManagedStoreDisk
	case ManagedStoreMemory:
		return datamodel.ManagedStoreMemory
	default:
		return datamodel.ManagedStoreDisk
	}
}

func fromManagedStoreDataModel(managedStore datamodel.ManagedStore) *ManagedStore {
	var m ManagedStore
	switch managedStore {
	case datamodel.ManagedStoreDisk:
		m = ManagedStoreDisk
	case datamodel.ManagedStoreMemory:
		m = ManagedStoreMemory
	default:
		m = ManagedStoreDisk
	}
	return &m
}

func toRbacDataModel(rbac *VolumeRbac) datamodel.VolumeRbac {
	switch *rbac {
	case VolumeRbacRead:
		return datamodel.VolumeRbacRead
	case VolumeRbacWrite:
		return datamodel.VolumeRbacWrite
	default:
		return datamodel.VolumeRbacRead
	}
}

func fromRbacDataModel(rbac datamodel.VolumeRbac) *VolumeRbac {
	var r VolumeRbac
	switch rbac {
	case datamodel.VolumeRbacRead:
		r = VolumeRbacRead
	case datamodel.VolumeRbacWrite:
		r = VolumeRbacWrite
	default:
		r = VolumeRbacRead
	}
	return &r
}

func toExtensionClassificationDataModel(e ExtensionClassification) datamodel.ExtensionClassification {
	switch c := e.(type) {
	case *ManualScalingExtension:
		converted := datamodel.ManualScalingExtension{
			Extension: datamodel.Extension{
				Kind: to.String(c.Extension.Kind),
			},
			Replicas: to.Int32(c.Replicas),
		}
		return converted
	case *DaprSidecarExtension:
		converted := datamodel.DaprSidecarExtension{
			Extension: datamodel.Extension{
				Kind: to.String(c.Extension.Kind),
			},
			AppID:    to.String(c.AppID),
			AppPort:  to.Int32(c.AppPort),
			Config:   to.String(c.Config),
			Protocol: toProtocolDataModel(c.Protocol),
			Provides: to.String(c.Provides),
		}
		return converted
	case *Extension:
		converted := datamodel.Extension{
			Kind: to.String(c.Kind),
		}
		return converted
	}
	return nil
}

func fromExtensionClassificationDataModel(e datamodel.ExtensionClassification) ExtensionClassification {
	switch c := e.(type) {
	case datamodel.ManualScalingExtension:
		converted := ManualScalingExtension{
			Extension: Extension{
				Kind: to.StringPtr(c.Extension.Kind),
			},
			Replicas: to.Int32Ptr(c.Replicas),
		}
		return converted.GetExtension()
	case datamodel.DaprSidecarExtension:
		converted := DaprSidecarExtension{
			Extension: Extension{
				Kind: to.StringPtr(c.Extension.Kind),
			},
			AppID:    to.StringPtr(c.AppID),
			AppPort:  to.Int32Ptr(c.AppPort),
			Config:   to.StringPtr(c.Config),
			Protocol: fromProtocolDataModel(c.Protocol),
			Provides: to.StringPtr(c.Provides),
		}
		return converted.GetExtension()
	case datamodel.Extension:
		converted := Extension{
			Kind: to.StringPtr(c.Kind),
		}
		return converted.GetExtension()
	}
	return nil
}

func toHealthProbeDataModelProperties(h HealthProbeProperties) datamodel.HealthProbeProperties {
	return datamodel.HealthProbeProperties{
		Kind:                to.String(h.Kind),
		FailureThreshold:    to.Float32(h.FailureThreshold),
		InitialDelaySeconds: to.Float32(h.InitialDelaySeconds),
		PeriodSeconds:       to.Float32(h.PeriodSeconds),
	}
}

func fromHealthProbeDataModelProperties(h datamodel.HealthProbeProperties) HealthProbeProperties {
	return HealthProbeProperties{
		Kind:                to.StringPtr(h.Kind),
		FailureThreshold:    to.Float32Ptr(h.FailureThreshold),
		InitialDelaySeconds: to.Float32Ptr(h.InitialDelaySeconds),
		PeriodSeconds:       to.Float32Ptr(h.PeriodSeconds),
	}
}

func toVolumeDataModel(c Volume) datamodel.Volume {
	return datamodel.Volume{
		Kind:      to.String(c.Kind),
		MountPath: to.String(c.MountPath),
	}
}

func fromVolumeDataModel(c datamodel.Volume) Volume {
	return Volume{
		Kind:      to.StringPtr(c.Kind),
		MountPath: to.StringPtr(c.MountPath),
	}
}
