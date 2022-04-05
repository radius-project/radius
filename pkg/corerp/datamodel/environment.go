// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/api/common"
)

// EnvironmentComputeKind is the type of compute resource.
type EnvironmentComputeKind string

const (
	// KubernetesComputeKind represents kubernetes compute resource type.
	KubernetesComputeKind EnvironmentComputeKind = "kubernetes"
)

// Environment represents Application environment resource.
type Environment struct {
	// ID is the fully qualified resource ID for the resource.
	ID string `json:"id"`
	// Name is the resource name.
	Name string `json:"name"`
	// Type is the resource type.
	Type string `json:"type"`
	// Location is the geo-location where resource is located.
	Location string `json:"location"`
	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`
	// Tags is the resource tags.
	Tags armrpcv1.ResourceTags `json:"tags,omitempty"`
	// Properties is the properties of the resource.
	Properties EnvironmentProperties `json:"properties"`
}

// EnvironmentProperties represents the properties of Environment.
type EnvironmentProperties struct {
	ProvisioningState common.ProvisioningStates `json:"provisioningState,omitempty"`
	Compute           EnvironmentCompute        `json:"compute,omitempty"`
}

// EnvironmentCompute represents the compute resource of Environment.
type EnvironmentCompute struct {
	Kind       EnvironmentComputeKind `json:"kind"`
	ResourceID string                 `json:"resourceId"`
}
