// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloads

import (
	"context"
	"errors"

	"github.com/Azure/radius/pkg/curp/components"
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
	Render(ctx context.Context, workload InstantiatedWorkload) ([]WorkloadResource, error)
}

// WorkloadResource represents the output of rendering a resource
type WorkloadResource struct {
	Type string
	// LocalID is just an identifier for the the workload processing logic to identify the resource
	LocalID  string
	Resource interface{}
}

// WorkloadResourceProperties represents the properties output by deploying a resource.
type WorkloadResourceProperties struct {
	Type       string
	Properties map[string]string
}

// WorkloadDispatcher defines the interface for locating a WorkloadRenderer based on the
// Kubernetes object type.
type WorkloadDispatcher interface {
	Lookup(kind string) (WorkloadRenderer, error)
}

// Dispatcher is an implementation of WorkloadDispatcher.
type Dispatcher struct {
	Renderers map[string]WorkloadRenderer
}

// NewKubernetesResource creates a Kubernetes WorkloadResource
func NewKubernetesResource(localID string, obj runtime.Object) WorkloadResource {
	return WorkloadResource{Type: ResourceKindKubernetes, LocalID: localID, Resource: obj}
}

func (wr WorkloadResource) IsKubernetesResource() bool {
	return wr.Type == ResourceKindKubernetes
}

// Lookup implements the WorkloadDispatcher contract.
func (d Dispatcher) Lookup(kind string) (WorkloadRenderer, error) {
	r, ok := d.Renderers[kind]
	if !ok {
		return nil, ErrUnknownType
	}

	return r, nil
}
