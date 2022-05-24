// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
)

// Container represents Container resource.
type Container struct {
	basedatamodel.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties ContainerProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	basedatamodel.InternalMetadata
}

// ResourceTypeName returns the qualified name of the resource
func (c Container) ResourceTypeName() string {
	return "Applications.Core/containers"
}

// ContainerProperties represents the properties of Container.
type ContainerProperties struct {
	ProvisioningState basedatamodel.ProvisioningStates `json:"provisioningState,omitempty"`
	Application       string                           `json:"application,omitempty"`
	Connections       map[string]ConnectionProperties  `json:"connections,omitempty"`
	Container         *Container                       `json:"container,omitempty"`
	Extensions        []ExtensionClassification        `json:"extensions,omitempty"`
}

// ConnectionProperties represents the properties of Connection.
type ConnectionProperties struct {
	Source                string        `json:"source,omitempty"`
	DisableDefaultEnvVars bool          `json:"disableDefaultEnvVars,omitempty"`
	Iam                   IamProperties `json:"iam,omitempty"`
}

// ExtensionClassification provides polymorphic access to related types.
// Call the interface's GetExtension() method to access the common type.
// Use a type switch to determine the concrete type.  The possible types are:
// - DaprSidecarExtension, Extension, ManualScalingExtension
type ExtensionClassification interface {
	GetExtension() Extension
}

// Extension of a resource.
type Extension struct {
	Kind string `json:"kind,omitempty"`
}

// Kind - The kind of IAM provider to configure
type Kind string

const (
	KindAzure Kind = "azure"
)

// IamProperties represents the properties of IAM provider.
type IamProperties struct {
	Kind  Kind     `json:"kind,omitempty"`
	Roles []string `json:"roles,omitempty"`
}
