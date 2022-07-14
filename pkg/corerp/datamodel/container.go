// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/rp"
)

// ContainerResource represents Container resource.
type ContainerResource struct {
	v1.TrackedResource

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties ContainerProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (c *ContainerResource) ResourceTypeName() string {
	return "Applications.Core/containers"
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (c *ContainerResource) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	c.Properties.Status.OutputResources = do.DeployedOutputResources
	c.InternalMetadata.ComputedValues = do.ComputedValues
	c.InternalMetadata.SecretValues = do.SecretValues
}

// OutputResources returns the output resources array.
func (c *ContainerResource) OutputResources() []outputresource.OutputResource {
	return c.Properties.Status.OutputResources
}

// ContainerProperties represents the properties of Container.
type ContainerProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState            `json:"provisioningState,omitempty"`
	Application       string                          `json:"application,omitempty"`
	Connections       map[string]ConnectionProperties `json:"connections,omitempty"`
	Container         Container                       `json:"container,omitempty"`
	Extensions        []Extension                     `json:"extensions,omitempty"`
}

// ConnectionProperties represents the properties of Connection.
type ConnectionProperties struct {
	Source                string        `json:"source,omitempty"`
	DisableDefaultEnvVars bool          `json:"disableDefaultEnvVars,omitempty"`
	IAM                   IAMProperties `json:"iam,omitempty"`
}

// Container - Definition of a container.
type Container struct {
	Image          string                      `json:"image,omitempty"`
	Env            map[string]string           `json:"env,omitempty"`
	LivenessProbe  HealthProbeProperties       `json:"livenessProbe,omitempty"`
	Ports          map[string]ContainerPort    `json:"ports,omitempty"`
	ReadinessProbe HealthProbeProperties       `json:"readinessProbe,omitempty"`
	Volumes        map[string]VolumeProperties `json:"volumes,omitempty"`
}

// ContainerPort - Specifies a listening port for the container
type ContainerPort struct {
	ContainerPort int32    `json:"containerPort,omitempty"`
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
	Source string     `json:"source,omitempty"`
	Rbac   VolumeRbac `json:"rbac,omitempty"`
}

// ManagedStore - Backing store for the ephemeral volume
type ManagedStore string

const (
	ManagedStoreDisk   ManagedStore = "disk"
	ManagedStoreMemory ManagedStore = "memory"
)

// VolumeRbac - Container read/write access to the volume
type VolumeRbac string

const (
	VolumeRbacRead  VolumeRbac = "read"
	VolumeRbacWrite VolumeRbac = "write"
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

// IsEmpty checks if the HealthProbeProperties is empty and returns true or false.
func (h HealthProbeProperties) IsEmpty() bool {
	return h == HealthProbeProperties{}
}

// HealthProbeBase - Properties for readiness/liveness probe
type HealthProbeBase struct {
	FailureThreshold    *float32 `json:"failureThreshold,omitempty"`
	InitialDelaySeconds *float32 `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       *float32 `json:"periodSeconds,omitempty"`
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

// ExtensionKind
type ExtensionKind string

const (
	ManualScaling ExtensionKind = "manualScaling"
	DaprSidecar   ExtensionKind = "daprSidecar"
)

// Extension of a resource.
type Extension struct {
	Kind          ExtensionKind           `json:"kind,omitempty"`
	ManualScaling *ManualScalingExtension `json:"manualScaling,omitempty"`
	DaprSidecar   *DaprSidecarExtension   `json:"daprSidecar,omitempty"`
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
	Provides string   `json:"provides,omitempty"`
}

type SecretObjectProperties struct {
	// REQUIRED; The name of the secret
	Name string `json:"name,omitempty"`

	// File name when written to disk.
	Alias string `json:"alias,omitempty"`

	// Encoding format. Default utf-8
	Encoding *SecretObjectPropertiesEncoding `json:"encoding,omitempty"`

	// Secret version
	Version string `json:"version,omitempty"`
}

// SecretObjectPropertiesEncoding - Encoding format. Default utf-8
type SecretObjectPropertiesEncoding string

const (
	SecretObjectPropertiesEncodingBase64 SecretObjectPropertiesEncoding = "base64"
	SecretObjectPropertiesEncodingHex    SecretObjectPropertiesEncoding = "hex"
	SecretObjectPropertiesEncodingUTF8   SecretObjectPropertiesEncoding = "utf-8"
)

type KeyObjectProperties struct {
	// REQUIRED; The name of the key
	Name string `json:"name,omitempty"`

	// File name when written to disk.
	Alias string `json:"alias,omitempty"`

	// Key version
	Version string `json:"version,omitempty"`
}

// IAMProperties represents the properties of IAM provider.
type IAMProperties struct {
	Kind  IAMKind  `json:"kind,omitempty"`
	Roles []string `json:"roles,omitempty"`
}

func (k IAMKind) IsValid() bool {
	s := Kinds()
	for _, v := range s {
		if v == k {
			return true
		}
	}
	return false
}

func (k IAMKind) IsKind(kind IAMKind) bool {
	return k == kind
}

// Kind - The kind of IAM provider to configure
type IAMKind string

const (
	KindAzure                   IAMKind = "azure"
	KindAzureComKeyVault        IAMKind = "azure.com/KeyVault"
	KindAzureComServiceBusQueue IAMKind = "azure.com/ServiceBusQueue"
	KindDaprIoInvokeHTTP        IAMKind = "dapr.io/InvokeHttp"
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

func Kinds() []IAMKind {
	return []IAMKind{
		KindAzure,
		KindAzureComKeyVault,
		KindAzureComServiceBusQueue,
		KindDaprIoInvokeHTTP,
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
