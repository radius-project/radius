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

package datamodel

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

const ContainerResourceType = "Applications.Core/containers"

// ContainerResource represents Container resource.
type ContainerResource struct {
	v1.BaseResource

	// TODO: remove this from CoreRP
	PortableResourceMetadata

	// Properties is the properties of the resource.
	Properties ContainerProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource.
func (c ContainerResource) ResourceTypeName() string {
	return ContainerResourceType
}

// ApplyDeploymentOutput updates the ContainerResource's Properties, ComputedValues and SecretValues with
// the DeploymentOutput's DeployedOutputResources, ComputedValues and SecretValues respectively and returns no error.
func (c *ContainerResource) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	c.Properties.Status.OutputResources = do.DeployedOutputResources
	c.ComputedValues = do.ComputedValues
	c.SecretValues = do.SecretValues
	return nil
}

// OutputResources returns the OutputResources from the ContainerResource's Properties Status.
func (c *ContainerResource) OutputResources() []rpv1.OutputResource {
	return c.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the ContainerResource instance.
func (h *ContainerResource) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// GetDisableDefaultEnvVars returns the value of the DisableDefaultEnvVars field of the ConnectionProperties struct, or
// false if the field is nil.
func (conn ConnectionProperties) GetDisableDefaultEnvVars() bool {
	if conn.DisableDefaultEnvVars == nil {
		return false
	}

	return *conn.DisableDefaultEnvVars
}

// ContainerProperties represents the properties of Container.
type ContainerProperties struct {
	rpv1.BasicResourceProperties
	Connections          map[string]ConnectionProperties `json:"connections,omitempty"`
	Container            Container                       `json:"container,omitempty"`
	Extensions           []Extension                     `json:"extensions,omitempty"`
	Identity             *rpv1.IdentitySettings          `json:"identity,omitempty"`
	Runtimes             *RuntimeProperties              `json:"runtimes,omitempty"`
	Resources            []ResourceReference             `json:"resources,omitempty"`
	ResourceProvisioning ContainerResourceProvisioning   `json:"resourceProvisioning,omitempty"`
}

// ContainerResourceProvisioning specifies how resources should be created for the container.
type ContainerResourceProvisioning string

const (
	// ContainerResourceProvisioningInternal specifies that Radius will create resources for the container according to its internal logic.
	ContainerResourceProvisioningInternal ContainerResourceProvisioning = "internal"

	// ContainerResourceProvisioningManual specifies that Radius will not create resources for the container, and the user will have to create them manually.
	ContainerResourceProvisioningManual ContainerResourceProvisioning = "manual"
)

// KubernetesRuntime represents the Kubernetes runtime configuration.
type KubernetesRuntime struct {
	// Base represents the Kubernetes resource definition in the serialized YAML format
	Base string `json:"base,omitempty"`

	// Pod represents the Kuberetes PodSpec strategic merge patch to be applied to the rendered PodSpec. This is stored as a JSON-encoded string.
	Pod string `json:"pod,omitempty"`
}

// RuntimeProperties represents the runtime configuration for the platform-specific functionalities.
type RuntimeProperties struct {
	// Kubernetes represents the Kubernetes runtime configuration.
	Kubernetes *KubernetesRuntime `json:"kubernetes,omitempty"`
}

// ConnectionProperties represents the properties of Connection.
type ConnectionProperties struct {
	Source                string        `json:"source,omitempty"`
	DisableDefaultEnvVars *bool         `json:"disableDefaultEnvVars,omitempty"`
	IAM                   IAMProperties `json:"iam,omitempty"`
}

// Container - Definition of a container.
type Container struct {
	Image           string                      `json:"image,omitempty"`
	ImagePullPolicy string                      `json:"imagePullPolicy,omitempty"`
	Env             map[string]string           `json:"env,omitempty"`
	LivenessProbe   HealthProbeProperties       `json:"livenessProbe,omitempty"`
	Ports           map[string]ContainerPort    `json:"ports,omitempty"`
	ReadinessProbe  HealthProbeProperties       `json:"readinessProbe,omitempty"`
	Volumes         map[string]VolumeProperties `json:"volumes,omitempty"`
	Command         []string                    `json:"command,omitempty"`
	Args            []string                    `json:"args,omitempty"`
	WorkingDir      string                      `json:"workingDir,omitempty"`
}

// ContainerPort - Specifies a listening port for the container
type ContainerPort struct {
	ContainerPort int32    `json:"containerPort,omitempty"`
	Port          int32    `json:"port,omitempty"`
	Scheme        string   `json:"scheme,omitempty"`
	Protocol      Protocol `json:"protocol,omitempty"`
	Provides      string   `json:"provides,omitempty"`
}

// Protocol - Protocol in use by the port
type Protocol string

const (
	ProtocolGrpc Protocol = "grpc"
	ProtocolHTTP Protocol = "http"
	ProtocolTCP  Protocol = "TCP"
	ProtocolUDP  Protocol = "UDP"
)

type ResourceReference struct {
	ID string `json:"id,omitempty"`
}

type VolumeKind string

const (
	Ephemeral  VolumeKind = "ephemeral"
	Persistent VolumeKind = "persistent"
)

// VolumeProperties - Specifies a volume for a container
type VolumeProperties struct {
	Kind       VolumeKind        `json:"kind,omitempty"`
	Ephemeral  *EphemeralVolume  `json:"ephemeralVolume,omitempty"`
	Persistent *PersistentVolume `json:"persistentVolume,omitempty"`
}

// Volume - Specifies a volume for a container
type VolumeBase struct {
	MountPath string `json:"mountPath,omitempty"`
}

// EphemeralVolume - Specifies an ephemeral volume for a container
type EphemeralVolume struct {
	VolumeBase
	ManagedStore ManagedStore `json:"managedStore,omitempty"`
}

// PersistentVolume - Specifies a persistent volume for a container
type PersistentVolume struct {
	VolumeBase
	Source     string           `json:"source,omitempty"`
	Permission VolumePermission `json:"permission,omitempty"`
}

// ManagedStore - Backing store for the ephemeral volume
type ManagedStore string

const (
	ManagedStoreDisk   ManagedStore = "disk"
	ManagedStoreMemory ManagedStore = "memory"
)

// VolumePermission - Container read/write access to the volume
type VolumePermission string

const (
	VolumePermissionRead  VolumePermission = "read"
	VolumePermissionWrite VolumePermission = "write"
)

type HealthProbeKind string

const (
	ExecHealthProbe    HealthProbeKind = "exec"
	HTTPGetHealthProbe HealthProbeKind = "httpGet"
	TCPHealthProbe     HealthProbeKind = "tcp"
)

// HealthProbeProperties - Properties for readiness/liveness probe
type HealthProbeProperties struct {
	Kind    HealthProbeKind               `json:"kind"`
	Exec    *ExecHealthProbeProperties    `json:"exec,omitempty"`
	HTTPGet *HTTPGetHealthProbeProperties `json:"httpGet,omitempty"`
	TCP     *TCPHealthProbeProperties     `json:"tcp,omitempty"`
}

// IsEmpty checks if the HealthProbeProperties is empty or not.
func (h HealthProbeProperties) IsEmpty() bool {
	return h == HealthProbeProperties{}
}

// HealthProbeBase - Properties for readiness/liveness probe
type HealthProbeBase struct {
	FailureThreshold    *float32 `json:"failureThreshold,omitempty"`
	InitialDelaySeconds *float32 `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       *float32 `json:"periodSeconds,omitempty"`
	TimeoutSeconds      *float32 `json:"timeoutSeconds,omitempty"`
}

// ExecHealthProbeProperties - Specifies the properties for readiness/liveness probe using an executable
type ExecHealthProbeProperties struct {
	HealthProbeBase
	Command string `json:"command,omitempty"`
}

// HTTPGetHealthProbeProperties - Specifies the properties for readiness/liveness probe using HTTP Get
type HTTPGetHealthProbeProperties struct {
	HealthProbeBase
	ContainerPort int32             `json:"containerPort,omitempty"`
	Path          string            `json:"path,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

// TCPHealthProbeProperties - Specifies the properties for readiness/liveness probe using TCP
type TCPHealthProbeProperties struct {
	HealthProbeBase
	ContainerPort int32 `json:"containerPort,omitempty"`
}

// ManualScalingExtension - ManualScaling Extension
type ManualScalingExtension struct {
	Replicas *int32 `json:"replicas,omitempty"`
}

// DaprSidecarExtension - Specifies the resource should have a Dapr sidecar injected
type DaprSidecarExtension struct {
	AppID    string   `json:"appId,omitempty"`
	AppPort  int32    `json:"appPort,omitempty"`
	Config   string   `json:"config,omitempty"`
	Protocol Protocol `json:"protocol,omitempty"`
}

// IAMProperties represents the properties of IAM provider.
type IAMProperties struct {
	Kind  IAMKind  `json:"kind,omitempty"`
	Roles []string `json:"roles,omitempty"`
}

// IsValid checks if the IAMKind is valid by comparing it to the list of valid IAMKinds.
func (k IAMKind) IsValid() bool {
	s := Kinds()
	for _, v := range s {
		if v == k {
			return true
		}
	}
	return false
}

// IsKind compares two IAMKinds and returns true if they are equal.
func (k IAMKind) IsKind(kind IAMKind) bool {
	return k == kind
}

// Kind - The kind of IAM provider to configure
type IAMKind string

const (
	KindAzure                   IAMKind = "azure"
	KindAzureComKeyVault        IAMKind = "azure.com/KeyVault"
	KindAzureComServiceBusQueue IAMKind = "azure.com/ServiceBusQueue"
	KindDaprIoPubSubTopic       IAMKind = "dapr.io/PubSubTopic"
	KindDaprIoSecretStore       IAMKind = "dapr.io/SecretStore"
	KindDaprIoStateStore        IAMKind = "dapr.io/StateStore"
	KindGrpc                    IAMKind = "Grpc"
	KindHTTP                    IAMKind = "Http"
	KindMicrosoftComSQL         IAMKind = "microsoft.com/SQL"
	KindMongoComMongoDB         IAMKind = "mongo.com/MongoDB"
	KindRabbitmqComMessageQueue IAMKind = "rabbitmq.com/MessageQueue"
	KindRedislabsComRedis       IAMKind = "redislabs.com/Redis"
)

// Kinds returns a list of supported IAMKinds.
func Kinds() []IAMKind {
	return []IAMKind{
		KindAzure,
		KindAzureComKeyVault,
		KindAzureComServiceBusQueue,
		KindDaprIoPubSubTopic,
		KindDaprIoSecretStore,
		KindDaprIoStateStore,
		KindGrpc,
		KindHTTP,
		KindMicrosoftComSQL,
		KindMongoComMongoDB,
		KindRabbitmqComMessageQueue,
		KindRedislabsComRedis,
	}
}
