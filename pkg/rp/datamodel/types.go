// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

// EnvironmentComputeKind is the type of compute resource.
type EnvironmentComputeKind string

const (
	// UnknownComputeKind represents kubernetes compute resource type.
	UnknownComputeKind EnvironmentComputeKind = "unknown"
	// KubernetesComputeKind represents kubernetes compute resource type.
	KubernetesComputeKind EnvironmentComputeKind = "kubernetes"
)

// BasicDaprResourceProperties is the basic resource properties for dapr resources.
type BasicDaprResourceProperties struct {
	// ComponentName represents the name of the component.
	ComponentName string `json:"componentName,omitempty"`
}

// BasicResourceProperties is the basic resource model for radius resources.
type BasicResourceProperties struct {
	// Environment represents the id of environment resource.
	Environment string `json:"environment,omitempty"`
	// Application represents the id of application resource.
	Application string `json:"application,omitempty"`

	// Status represents the resource status.
	Status ResourceStatus `json:"status,omitempty"`
}

// EqualLinkedResource returns true if the resource belongs to the same environment and application.
func (b *BasicResourceProperties) EqualLinkedResource(prop *BasicResourceProperties) bool {
	return strings.EqualFold(b.Application, prop.Application) && strings.EqualFold(b.Environment, prop.Environment)
}

type ResourceStatus struct {
	Compute         *EnvironmentCompute             `json:"compute,omitempty"`
	OutputResources []outputresource.OutputResource `json:"outputResources,omitempty"`
}

func (in *ResourceStatus) DeepCopy(out *ResourceStatus) {
	in.Compute = out.Compute
	in.OutputResources = out.OutputResources
}

// EnvironmentCompute represents the compute resource of Environment.
type EnvironmentCompute struct {
	Kind              EnvironmentComputeKind      `json:"kind"`
	KubernetesCompute KubernetesComputeProperties `json:"kubernetes,omitempty"`

	// Environment-level identity that can be used by any resource in the environment.
	// Resources can specify its own identities and they will override the environment-level identity.
	Identity *IdentitySettings `json:"identity,omitempty"`
}

// KubernetesComputeProperties represents the kubernetes compute of the environment.
type KubernetesComputeProperties struct {
	// ResourceID represents the resource ID for kuberentes compute resource.
	ResourceID string `json:"resourceId,omitempty"`

	// Namespace represents Kubernetes namespace.
	Namespace string `json:"namespace"`
}

// RadiusResourceModel represents the interface of radius resource type.
// TODO: Replace DeploymentDataModel with RadiusResourceModel later when link rp leverages generic.
type RadiusResourceModel interface {
	v1.ResourceDataModel

	ApplyDeploymentOutput(deploymentOutput outputresource.DeploymentOutput)
	OutputResources() []outputresource.OutputResource

	ResourceMetadata() *BasicResourceProperties
}
