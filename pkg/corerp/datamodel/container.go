// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// ContainerResource represents Container resource.
type ContainerResource struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties ContainerProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

// ResourceTypeName returns the qualified name of the resource
func (c ContainerResource) ResourceTypeName() string {
	return "Applications.Core/containers"
}

// ContainerProperties represents the properties of Container.
type ContainerProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState            `json:"provisioningState,omitempty"`
	Application       string                          `json:"application,omitempty"`
	Connections       map[string]ConnectionProperties `json:"connections,omitempty"`
	Container         Container                       `json:"container,omitempty"`
	Extensions        []ExtensionClassification       `json:"extensions,omitempty"`
}

// ConnectionProperties represents the properties of Connection.
type ConnectionProperties struct {
	Source                string        `json:"source,omitempty"`
	DisableDefaultEnvVars bool          `json:"disableDefaultEnvVars,omitempty"`
	Iam                   IamProperties `json:"iam,omitempty"`
}

// Container - Definition of a container.
type Container struct {
	Image          string                              `json:"image,omitempty"`
	Env            map[string]string                   `json:"env,omitempty"`
	LivenessProbe  HealthProbePropertiesClassification `json:"livenessProbe,omitempty"`
	Ports          map[string]ContainerPort            `json:"ports,omitempty"`
	ReadinessProbe HealthProbePropertiesClassification `json:"readinessProbe,omitempty"`
	Volumes        map[string]VolumeClassification     `json:"volumes,omitempty"`
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

// VolumeClassification provides polymorphic access to related types.
type VolumeClassification interface {
	GetVolume() Volume
}

// Volume - Specifies a volume for a container
type Volume struct {
	Kind      string `json:"kind,omitempty"`
	MountPath string `json:"mountPath,omitempty"`
}

// EphemeralVolume - Specifies an ephemeral volume for a container
type EphemeralVolume struct {
	Volume
	ManagedStore ManagedStore `json:"managedStore,omitempty"`
}

// PersistentVolume - Specifies a persistent volume for a container
type PersistentVolume struct {
	Volume
	Source string     `json:"source,omitempty"`
	Rbac   VolumeRbac `json:"rbac,omitempty"`
}

// GetVolume implements the VolumeClassification interface for type Volume.
func (v Volume) GetVolume() Volume { return v }

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

type HealthProbePropertiesClassification interface {
	GetHealthProbeProperties() *HealthProbeProperties
}

// HealthProbeProperties - Properties for readiness/liveness probe
type HealthProbeProperties struct {
	Kind                string   `json:"kind,omitempty"`
	FailureThreshold    *float32 `json:"failureThreshold,omitempty"`
	InitialDelaySeconds *float32 `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       *float32 `json:"periodSeconds,omitempty"`
}

// ExecHealthProbeProperties - Specifies the properties for readiness/liveness probe using an executable
type ExecHealthProbeProperties struct {
	HealthProbeProperties
	Command string `json:"command,omitempty"`
}

// HTTPGetHealthProbeProperties - Specifies the properties for readiness/liveness probe using HTTP Get
type HTTPGetHealthProbeProperties struct {
	HealthProbeProperties
	ContainerPort int32             `json:"containerPort,omitempty"`
	Path          string            `json:"path,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

// TCPHealthProbeProperties - Specifies the properties for readiness/liveness probe using TCP
type TCPHealthProbeProperties struct {
	HealthProbeProperties
	ContainerPort int32 `json:"containerPort,omitempty"`
}

func (h *HealthProbeProperties) GetHealthProbeProperties() *HealthProbeProperties {
	return h
}

// ExtensionClassification provides polymorphic access to related types.
// Call the interface's GetExtension() method to access the common type.
// Use a type switch to determine the concrete type.  The possible types are:
// - DaprSidecarExtension, Extension, ManualScalingExtension
type ExtensionClassification interface {
	GetExtension() Extension
}

// ManualScalingExtension - ManualScaling Extension
type ManualScalingExtension struct {
	Extension
	Replicas int32 `json:"replicas,omitempty"`
}

// DaprSidecarExtension - Specifies the resource should have a Dapr sidecar injected
type DaprSidecarExtension struct {
	Extension
	AppID    string   `json:"appId,omitempty"`
	AppPort  int32    `json:"appPort,omitempty"`
	Config   string   `json:"config,omitempty"`
	Protocol Protocol `json:"protocol,omitempty"`
	Provides string   `json:"provides,omitempty"`
}

// Extension of a resource.
type Extension struct {
	Kind string `json:"kind,omitempty"`
}

// GetExtension implements the ExtensionClassification interface for type Extension.
func (e Extension) GetExtension() Extension { return e }

// Kind - The kind of IAM provider to configure
type Kind string

const (
	KindAzure                   Kind = "azure"
	KindAzureComKeyVault        Kind = "azure.com/KeyVault"
	KindAzureComServiceBusQueue Kind = "azure.com/ServiceBusQueue"
	KindDaprIoInvokeHTTP        Kind = "dapr.io/InvokeHttp"
	KindDaprIoPubSubTopic       Kind = "dapr.io/PubSubTopic"
	KindDaprIoSecretStore       Kind = "dapr.io/SecretStore"
	KindDaprIoStateStore        Kind = "dapr.io/StateStore"
	KindGrpc                    Kind = "Grpc"
	KindHTTP                    Kind = "Http"
	KindMicrosoftComSQL         Kind = "microsoft.com/SQL"
	KindMongoComMongoDB         Kind = "mongo.com/MongoDB"
	KindRabbitmqComMessageQueue Kind = "rabbitmq.com/MessageQueue"
	KindRedislabsComRedis       Kind = "redislabs.com/Redis"
)

// Kinds returns the possible values for the Kind const type.
func Kinds() []Kind {
	return []Kind{
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

func (k Kind) IsValid() bool {
	s := Kinds()
	for _, v := range s {
		if v == k {
			return true
		}
	}
	return false
}

func (k Kind) IsKind(kind Kind) bool {
	return k == kind
}

type SecretObjectProperties struct {
	// REQUIRED; The name of the secret
	Name *string `json:"name,omitempty"`

	// File name when written to disk.
	Alias *string `json:"alias,omitempty"`

	// Encoding format. Default utf-8
	Encoding *SecretObjectPropertiesEncoding `json:"encoding,omitempty"`

	// Secret version
	Version *string `json:"version,omitempty"`
}

// SecretObjectPropertiesEncoding - Encoding format. Default utf-8
type SecretObjectPropertiesEncoding string

const (
	SecretObjectPropertiesEncodingBase64 SecretObjectPropertiesEncoding = "base64"
	SecretObjectPropertiesEncodingHex    SecretObjectPropertiesEncoding = "hex"
	SecretObjectPropertiesEncodingUTF8   SecretObjectPropertiesEncoding = "utf-8"
)

// SecretObjectPropertiesEncodingValues returns the possible values for the SecretObjectPropertiesEncoding const type.
func SecretObjectPropertiesEncodingValues() []SecretObjectPropertiesEncoding {
	return []SecretObjectPropertiesEncoding{
		SecretObjectPropertiesEncodingBase64,
		SecretObjectPropertiesEncodingHex,
		SecretObjectPropertiesEncodingUTF8,
	}
}

type KeyObjectProperties struct {
	// REQUIRED; The name of the key
	Name *string `json:"name,omitempty"`

	// File name when written to disk.
	Alias *string `json:"alias,omitempty"`

	// Key version
	Version *string `json:"version,omitempty"`
}

// IamProperties represents the properties of IAM provider.
type IamProperties struct {
	Kind  Kind     `json:"kind,omitempty"`
	Roles []string `json:"roles,omitempty"`
}
