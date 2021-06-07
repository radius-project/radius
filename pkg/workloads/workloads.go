// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloads

import (
	"context"
	"errors"
	"strings"

	"github.com/Azure/radius/pkg/radrp/components"
	"k8s.io/apimachinery/pkg/runtime"
)

// ErrUnknownType is the error reported when the workload type is unknown or unsupported.
var ErrUnknownType = errors.New("workload type is unsupported")

// InstantiatedWorkload workload provides all of the information needed to render a workload.
type InstantiatedWorkload struct {
	Application   string
	Name          string
	Workload      components.GenericComponent
	BindingValues map[components.BindingKey]components.BindingState
}

// WorkloadRenderer defines the interface for rendering a Kubernetes workload definition
// into a set of raw Kubernetes resources.
//
// The idea is that this represents *fan-out* in terms of the implementation. All of the APIs here
// could be replaced with REST calls.
type WorkloadRenderer interface {
	// AllocateBindings is called for the component to provide its supported bindings and their values.
	AllocateBindings(ctx context.Context, workload InstantiatedWorkload, resources []WorkloadResourceProperties) (map[string]components.BindingState, error)
	// Render is called for the component to provide its output resources.
	Render(ctx context.Context, workload InstantiatedWorkload) ([]OutputResource, error)
}

// WorkloadResourceProperties represents the properties output by deploying a resource.
type WorkloadResourceProperties struct {
	Type       string
	Properties map[string]string
}

// NewKubernetesResource creates a Kubernetes WorkloadResource
func NewKubernetesResource(localID string, obj runtime.Object) OutputResource {
	return OutputResource{ResourceKind: ResourceKindKubernetes, LocalID: localID, Resource: obj}
}

func (wr OutputResource) IsKubernetesResource() bool {
	return wr.ResourceKind == ResourceKindKubernetes
}

// GetOutputResourceType determines the deployment resource type
func (wr OutputResource) GetOutputResourceType() string {
	if wr.ResourceKind == ResourceKindAzurePodIdentity {
		return OutputResourceTypePodIdentity
	} else if strings.Contains(wr.ResourceKind, "azure") {
		return OutputResourceTypeArm
	} else if wr.ResourceKind == ResourceKindKubernetes {
		return OutputResourceTypeKubernetes
	} else {
		return ""
	}
}
