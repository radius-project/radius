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
	"encoding/json"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned Container resource to version-agnostic datamodel.
func (src *ContainerResource) ConvertTo() (v1.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.

	connections := make(map[string]datamodel.ConnectionProperties)
	for key, val := range src.Properties.Connections {
		if val != nil {
			roles := []string{}
			var kind datamodel.IAMKind

			if val.Iam != nil {
				for _, r := range val.Iam.Roles {
					roles = append(roles, to.String(r))
				}
				kind = toKindDataModel(val.Iam.Kind)
			}

			var disableDefaultEnvVars bool
			if val.DisableDefaultEnvVars != nil {
				disableDefaultEnvVars = to.Bool(val.DisableDefaultEnvVars)
			}

			connections[key] = datamodel.ConnectionProperties{
				Source:                to.String(val.Source),
				DisableDefaultEnvVars: &disableDefaultEnvVars,
				IAM: datamodel.IAMProperties{
					Kind:  kind,
					Roles: roles,
				},
			}
		}
	}

	var livenessProbe datamodel.HealthProbeProperties
	if src.Properties.Container.LivenessProbe != nil {
		livenessProbe = toHealthProbePropertiesDataModel(src.Properties.Container.LivenessProbe)
	}

	var readinessProbe datamodel.HealthProbeProperties
	if src.Properties.Container.ReadinessProbe != nil {
		readinessProbe = toHealthProbePropertiesDataModel(src.Properties.Container.ReadinessProbe)
	}

	ports := make(map[string]datamodel.ContainerPort)
	for key, val := range src.Properties.Container.Ports {
		port := datamodel.ContainerPort{
			ContainerPort: to.Int32(val.ContainerPort),
			Protocol:      toPortProtocolDataModel(val.Protocol),
			Provides:      to.String(val.Provides),
		}

		if val.Port != nil {
			port.Port = to.Int32(val.Port)
		}

		if val.Scheme != nil {
			port.Scheme = to.String(val.Scheme)
		}

		ports[key] = port
	}

	var volumes map[string]datamodel.VolumeProperties
	if src.Properties.Container.Volumes != nil {
		volumes = make(map[string]datamodel.VolumeProperties)
		for key, val := range src.Properties.Container.Volumes {
			volumes[key] = toVolumePropertiesDataModel(val)
		}
	}

	var extensions []datamodel.Extension
	if src.Properties.Extensions != nil {
		for _, e := range src.Properties.Extensions {
			extensions = append(extensions, toExtensionDataModel(e))
		}
	}

	converted := &datamodel.ContainerResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: datamodel.ContainerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: to.String(src.Properties.Application),
			},
			Connections: connections,
			Container: datamodel.Container{
				Image:           to.String(src.Properties.Container.Image),
				ImagePullPolicy: toImagePullPolicyDataModel(src.Properties.Container.ImagePullPolicy),
				Env:             to.StringMap(src.Properties.Container.Env),
				LivenessProbe:   livenessProbe,
				Ports:           ports,
				ReadinessProbe:  readinessProbe,
				Volumes:         volumes,
				Command:         stringSlice(src.Properties.Container.Command),
				Args:            stringSlice(src.Properties.Container.Args),
				WorkingDir:      to.String(src.Properties.Container.WorkingDir),
			},
			Extensions: extensions,
			Runtimes:   toRuntimeProperties(src.Properties.Runtimes),
		},
	}

	if src.Properties.Identity != nil {
		converted.Properties.Identity = &rpv1.IdentitySettings{
			Kind:       toIdentityKind(src.Properties.Identity.Kind),
			OIDCIssuer: to.String(src.Properties.Identity.OidcIssuer),
			Resource:   to.String(src.Properties.Identity.Resource),
		}
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Container resource.
func (dst *ContainerResource) ConvertFrom(src v1.DataModelInterface) error {
	c, ok := src.(*datamodel.ContainerResource)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	connections := make(map[string]*ConnectionProperties)
	for key, val := range c.Properties.Connections {
		roles := []*string{}
		var kind *IAMKind

		for _, r := range val.IAM.Roles {
			roles = append(roles, to.Ptr(r))
		}

		kind = fromKindDataModel(val.IAM.Kind)

		var disableDefaultEnvVars bool
		if val.DisableDefaultEnvVars != nil {
			disableDefaultEnvVars = to.Bool(val.DisableDefaultEnvVars)
		}

		connections[key] = &ConnectionProperties{
			Source:                to.Ptr(val.Source),
			DisableDefaultEnvVars: &disableDefaultEnvVars,
			Iam: &IamProperties{
				Kind:  kind,
				Roles: roles,
			},
		}
	}

	var livenessProbe HealthProbePropertiesClassification
	if !c.Properties.Container.LivenessProbe.IsEmpty() {
		livenessProbe = fromHealthProbePropertiesDataModel(c.Properties.Container.LivenessProbe)
	}

	var readinessProbe HealthProbePropertiesClassification
	if !c.Properties.Container.ReadinessProbe.IsEmpty() {
		readinessProbe = fromHealthProbePropertiesDataModel(c.Properties.Container.ReadinessProbe)
	}

	ports := make(map[string]*ContainerPortProperties)
	for key, val := range c.Properties.Container.Ports {
		ports[key] = &ContainerPortProperties{
			ContainerPort: to.Ptr(val.ContainerPort),
			Protocol:      fromPortProtocolDataModel(val.Protocol),
			Provides:      to.Ptr(val.Provides),
		}

		if val.Port != 0 {
			ports[key].Port = to.Ptr(val.Port)
		}

		if val.Scheme != "" {
			ports[key].Scheme = to.Ptr(val.Scheme)
		}
	}

	var volumes map[string]VolumeClassification
	if c.Properties.Container.Volumes != nil {
		volumes = make(map[string]VolumeClassification)
		for key, val := range c.Properties.Container.Volumes {
			volumes[key] = fromVolumePropertiesDataModel(val)
		}
	}

	var extensions []ExtensionClassification
	if c.Properties.Extensions != nil {
		for _, e := range c.Properties.Extensions {
			extensions = append(extensions, fromExtensionClassificationDataModel(e))
		}
	}

	var identity *IdentitySettings
	if c.Properties.Identity != nil {
		identity = &IdentitySettings{
			Kind:       fromIdentityKind(c.Properties.Identity.Kind),
			Resource:   to.Ptr(c.Properties.Identity.Resource),
			OidcIssuer: to.Ptr(c.Properties.Identity.OIDCIssuer),
		}
	}

	dst.ID = to.Ptr(c.ID)
	dst.Name = to.Ptr(c.Name)
	dst.Type = to.Ptr(c.Type)
	dst.SystemData = fromSystemDataModel(c.SystemData)
	dst.Location = to.Ptr(c.Location)
	dst.Tags = *to.StringMapPtr(c.Tags)
	dst.Properties = &ContainerProperties{
		Status: &ResourceStatus{
			OutputResources: toOutputResources(c.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(c.InternalMetadata.AsyncProvisioningState),
		Application:       to.Ptr(c.Properties.Application),
		Connections:       connections,
		Container: &Container{
			Image:           to.Ptr(c.Properties.Container.Image),
			ImagePullPolicy: fromImagePullPolicyDataModel(c.Properties.Container.ImagePullPolicy),
			Env:             *to.StringMapPtr(c.Properties.Container.Env),
			LivenessProbe:   livenessProbe,
			Ports:           ports,
			ReadinessProbe:  readinessProbe,
			Volumes:         volumes,
			Command:         to.SliceOfPtrs(c.Properties.Container.Command...),
			Args:            to.SliceOfPtrs(c.Properties.Container.Args...),
			WorkingDir:      to.Ptr(c.Properties.Container.WorkingDir),
		},
		Extensions: extensions,
		Identity:   identity,
		Runtimes:   fromRuntimeProperties(c.Properties.Runtimes),
	}

	return nil
}

func toImagePullPolicyDataModel(pullPolicy *ImagePullPolicy) string {
	if pullPolicy == nil {
		return ""
	}

	switch *pullPolicy {
	case ImagePullPolicyAlways:
		return "Always"
	case ImagePullPolicyIfNotPresent:
		return "IfNotPresent"
	case ImagePullPolicyNever:
		return "Never"
	default:
		return ""
	}
}

func fromImagePullPolicyDataModel(pullPolicy string) *ImagePullPolicy {
	switch pullPolicy {
	case "Always":
		return to.Ptr(ImagePullPolicyAlways)
	case "IfNotPresent":
		return to.Ptr(ImagePullPolicyIfNotPresent)
	case "Never":
		return to.Ptr(ImagePullPolicyNever)
	default:
		return nil
	}
}

func toHealthProbePropertiesDataModel(h HealthProbePropertiesClassification) datamodel.HealthProbeProperties {
	switch c := h.(type) {
	case *ExecHealthProbeProperties:
		return datamodel.HealthProbeProperties{
			Kind: datamodel.ExecHealthProbe,
			Exec: &datamodel.ExecHealthProbeProperties{
				HealthProbeBase: toHealthProbeBase(*c.GetHealthProbeProperties()),
				Command:         to.String(c.Command),
			},
		}
	case *HTTPGetHealthProbeProperties:
		return datamodel.HealthProbeProperties{
			Kind: datamodel.HTTPGetHealthProbe,
			HTTPGet: &datamodel.HTTPGetHealthProbeProperties{
				HealthProbeBase: toHealthProbeBase(*c.GetHealthProbeProperties()),
				ContainerPort:   to.Int32(c.ContainerPort),
				Path:            to.String(c.Path),
				Headers:         to.StringMap(c.Headers),
			},
		}
	case *TCPHealthProbeProperties:
		return datamodel.HealthProbeProperties{
			Kind: datamodel.TCPHealthProbe,
			TCP: &datamodel.TCPHealthProbeProperties{
				HealthProbeBase: toHealthProbeBase(*c.GetHealthProbeProperties()),
				ContainerPort:   to.Int32(c.ContainerPort),
			},
		}
	}

	return datamodel.HealthProbeProperties{}
}

func fromHealthProbePropertiesDataModel(h datamodel.HealthProbeProperties) HealthProbePropertiesClassification {
	switch h.Kind {
	case datamodel.ExecHealthProbe:
		return &ExecHealthProbeProperties{
			Kind:                (*string)(&h.Kind),
			FailureThreshold:    h.Exec.FailureThreshold,
			InitialDelaySeconds: h.Exec.InitialDelaySeconds,
			PeriodSeconds:       h.Exec.PeriodSeconds,
			TimeoutSeconds:      h.Exec.TimeoutSeconds,
			Command:             to.Ptr(h.Exec.Command),
		}
	case datamodel.HTTPGetHealthProbe:
		return &HTTPGetHealthProbeProperties{
			Kind:                (*string)(&h.Kind),
			FailureThreshold:    h.HTTPGet.FailureThreshold,
			InitialDelaySeconds: h.HTTPGet.InitialDelaySeconds,
			PeriodSeconds:       h.HTTPGet.PeriodSeconds,
			TimeoutSeconds:      h.HTTPGet.TimeoutSeconds,
			ContainerPort:       to.Ptr(h.HTTPGet.ContainerPort),
			Path:                to.Ptr(h.HTTPGet.Path),
			Headers:             *to.StringMapPtr(h.HTTPGet.Headers),
		}
	case datamodel.TCPHealthProbe:
		return &TCPHealthProbeProperties{
			Kind:                (*string)(&h.Kind),
			FailureThreshold:    h.TCP.FailureThreshold,
			InitialDelaySeconds: h.TCP.InitialDelaySeconds,
			PeriodSeconds:       h.TCP.PeriodSeconds,
			TimeoutSeconds:      h.TCP.TimeoutSeconds,
			ContainerPort:       to.Ptr(h.TCP.ContainerPort),
		}
	}

	return nil
}

func toKindDataModel(kind *IAMKind) datamodel.IAMKind {
	switch *kind {
	case IAMKindAzure:
		return datamodel.KindAzure
	default:
		return datamodel.KindAzure
	}
}

func fromKindDataModel(kind datamodel.IAMKind) *IAMKind {
	var k IAMKind
	switch kind {
	case datamodel.KindAzure:
		k = IAMKindAzure
	default:
		k = IAMKindAzure
	}
	return &k
}

func toPortProtocolDataModel(protocol *PortProtocol) datamodel.Protocol {
	if protocol == nil {
		return datamodel.ProtocolTCP
	}
	switch *protocol {
	case PortProtocolTCP:
		return datamodel.ProtocolTCP
	case PortProtocolUDP:
		return datamodel.ProtocolUDP
	default:
		return datamodel.ProtocolTCP
	}
}

func toDaprProtocolDataModel(protocol *DaprSidecarExtensionProtocol) datamodel.Protocol {
	if protocol == nil {
		return datamodel.ProtocolHTTP
	}
	switch *protocol {
	case DaprSidecarExtensionProtocolHTTP:
		return datamodel.ProtocolHTTP
	case DaprSidecarExtensionProtocolGrpc:
		return datamodel.ProtocolGrpc
	default:
		return datamodel.ProtocolHTTP
	}
}
func fromPortProtocolDataModel(protocol datamodel.Protocol) *PortProtocol {
	var p PortProtocol
	switch protocol {
	case datamodel.ProtocolTCP:
		p = PortProtocolTCP
	case datamodel.ProtocolUDP:
		p = PortProtocolUDP
	default:
		p = PortProtocolTCP
	}
	return &p
}

func fromProtocolDataModel(protocol datamodel.Protocol) *DaprSidecarExtensionProtocol {
	var p DaprSidecarExtensionProtocol
	switch protocol {
	case datamodel.ProtocolGrpc:
		p = DaprSidecarExtensionProtocolGrpc
	default:
		p = DaprSidecarExtensionProtocolHTTP
	}
	return &p
}

func toVolumePropertiesDataModel(h VolumeClassification) datamodel.VolumeProperties {
	switch c := h.(type) {
	case *EphemeralVolume:
		return datamodel.VolumeProperties{
			Kind: datamodel.Ephemeral,
			Ephemeral: &datamodel.EphemeralVolume{
				VolumeBase:   toVolumeBaseDataModel(*c.GetVolume()),
				ManagedStore: toManagedStoreDataModel(c.ManagedStore),
			},
		}
	case *PersistentVolume:
		return datamodel.VolumeProperties{
			Kind: datamodel.Persistent,
			Persistent: &datamodel.PersistentVolume{
				VolumeBase: toVolumeBaseDataModel(*c.GetVolume()),
				Source:     to.String(c.Source),
				Permission: toPermissionDataModel(c.Permission),
			},
		}
	}

	return datamodel.VolumeProperties{}
}

func fromVolumePropertiesDataModel(v datamodel.VolumeProperties) VolumeClassification {
	switch v.Kind {
	case datamodel.Ephemeral:
		return &EphemeralVolume{
			Kind:         (*string)(&v.Kind),
			MountPath:    &v.Ephemeral.MountPath,
			ManagedStore: fromManagedStoreDataModel(v.Ephemeral.ManagedStore),
		}
	case datamodel.Persistent:
		return &PersistentVolume{
			Kind:       (*string)(&v.Kind),
			MountPath:  &v.Persistent.MountPath,
			Source:     &v.Persistent.Source,
			Permission: fromPermissionDataModel(v.Persistent.Permission),
		}
	}

	return nil
}

func toManagedStoreDataModel(ms *ManagedStore) datamodel.ManagedStore {
	switch *ms {
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

func toRuntimeProperties(runtime *RuntimesProperties) *datamodel.RuntimeProperties {
	if runtime == nil {
		return nil
	}

	r := &datamodel.RuntimeProperties{}
	if runtime.Kubernetes != nil {
		r.Kubernetes = &datamodel.KubernetesRuntime{
			Base: to.String(runtime.Kubernetes.Base),
		}
		if runtime.Kubernetes.Pod != nil {
			// Serializes PodSpec patch object to JSON-encoded. Internally, Radius does JSON strategic merge patch
			// with this JSON-encoded PodSpec patch object. Thus, datamodel holds JSON-encoded PodSpec patch object
			// as a string.
			serialiedPodPatch, err := json.Marshal(runtime.Kubernetes.Pod)
			if err != nil {
				return nil
			}
			r.Kubernetes.Pod = string(serialiedPodPatch)
		}
	}
	return r
}

func fromRuntimeProperties(runtime *datamodel.RuntimeProperties) *RuntimesProperties {
	if runtime == nil {
		return nil
	}
	r := &RuntimesProperties{}
	if runtime.Kubernetes != nil {
		r.Kubernetes = &KubernetesRuntimeProperties{
			Base: to.Ptr(runtime.Kubernetes.Base),
		}
		if runtime.Kubernetes.Pod != "" {
			podPatch := map[string]any{}
			if err := json.Unmarshal([]byte(runtime.Kubernetes.Pod), &podPatch); err != nil {
				return nil
			}
			r.Kubernetes.Pod = podPatch
		}
	}
	return r
}

func toPermissionDataModel(rbac *VolumePermission) datamodel.VolumePermission {
	if rbac == nil {
		return datamodel.VolumePermissionRead
	}

	switch *rbac {
	case VolumePermissionRead:
		return datamodel.VolumePermissionRead
	case VolumePermissionWrite:
		return datamodel.VolumePermissionWrite
	default:
		return datamodel.VolumePermissionRead
	}
}

func fromPermissionDataModel(rbac datamodel.VolumePermission) *VolumePermission {
	var r VolumePermission
	switch rbac {
	case datamodel.VolumePermissionRead:
		r = VolumePermissionRead
	case datamodel.VolumePermissionWrite:
		r = VolumePermissionWrite
	default:
		r = VolumePermissionRead
	}
	return &r
}

// toExtensionDataModel: Converts from versioned datamodel to base datamodel
func toExtensionDataModel(e ExtensionClassification) datamodel.Extension {
	switch c := e.(type) {
	case *ManualScalingExtension:
		return datamodel.Extension{
			Kind: datamodel.ManualScaling,
			ManualScaling: &datamodel.ManualScalingExtension{
				Replicas: c.Replicas,
			},
		}
	case *DaprSidecarExtension:
		return datamodel.Extension{
			Kind: datamodel.DaprSidecar,
			DaprSidecar: &datamodel.DaprSidecarExtension{
				AppID:    to.String(c.AppID),
				AppPort:  to.Int32(c.AppPort),
				Config:   to.String(c.Config),
				Protocol: toDaprProtocolDataModel(c.Protocol),
			},
		}
	case *KubernetesMetadataExtension:
		return datamodel.Extension{
			Kind: datamodel.KubernetesMetadata,
			KubernetesMetadata: &datamodel.KubeMetadataExtension{
				Annotations: to.StringMap(c.Annotations),
				Labels:      to.StringMap(c.Labels),
			},
		}
	}

	return datamodel.Extension{}
}

// fromExtensionClassificationDataModel: Converts from base datamodel to versioned datamodel
func fromExtensionClassificationDataModel(e datamodel.Extension) ExtensionClassification {
	switch e.Kind {
	case datamodel.ManualScaling:
		return &ManualScalingExtension{
			Kind:     to.Ptr(string(e.Kind)),
			Replicas: e.ManualScaling.Replicas,
		}
	case datamodel.DaprSidecar:
		return &DaprSidecarExtension{
			Kind:     to.Ptr(string(e.Kind)),
			AppID:    to.Ptr(e.DaprSidecar.AppID),
			AppPort:  to.Ptr(e.DaprSidecar.AppPort),
			Config:   to.Ptr(e.DaprSidecar.Config),
			Protocol: fromProtocolDataModel(e.DaprSidecar.Protocol),
		}
	case datamodel.KubernetesMetadata:
		var ann, lbl = fromExtensionClassificationFields(e)
		return &KubernetesMetadataExtension{
			Kind:        to.Ptr(string(e.Kind)),
			Annotations: *to.StringMapPtr(ann),
			Labels:      *to.StringMapPtr(lbl),
		}
	}

	return nil
}

func toHealthProbeBase(h HealthProbeProperties) datamodel.HealthProbeBase {
	return datamodel.HealthProbeBase{
		FailureThreshold:    h.FailureThreshold,
		InitialDelaySeconds: h.InitialDelaySeconds,
		PeriodSeconds:       h.PeriodSeconds,
		TimeoutSeconds:      h.TimeoutSeconds,
	}
}

func toVolumeBaseDataModel(v Volume) datamodel.VolumeBase {
	return datamodel.VolumeBase{
		MountPath: *v.MountPath,
	}
}

func fromExtensionClassificationFields(e datamodel.Extension) (map[string]string, map[string]string) {
	var ann map[string]string
	var lbl map[string]string

	if e.KubernetesMetadata != nil {
		if e.KubernetesMetadata.Annotations != nil {
			ann = e.KubernetesMetadata.Annotations
		}
		if e.KubernetesMetadata.Labels != nil {
			lbl = e.KubernetesMetadata.Labels
		}
	}

	return ann, lbl
}
