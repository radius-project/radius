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
	ServiceValues map[string]map[string]interface{}
	Traits        []WorkloadTrait
	Provides      map[string]map[string]interface{}
	// TODO scopes go here.
}

// WorkloadService represents a service that the workload provides.
type WorkloadService struct {
	Name string
	Kind string
}

// WorkloadTrait represents the trait data made available to a workload.
type WorkloadTrait struct {
	Kind       string
	Properties map[string]interface{}
}

// WorkloadRenderer defines the interface for rendering a Kubernetes workload definition
// into a set of raw Kubernetes resources.
//
// The idea is that this represents *fan-out* in terms of the implementation. All of the APIs here
// could be replaced with REST calls.
type WorkloadRenderer interface {
	Allocate(ctx context.Context, workload InstantiatedWorkload, resources []WorkloadResourceProperties, service WorkloadService) (map[string]interface{}, error)
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
	return WorkloadResource{Type: "kubernetes", LocalID: localID, Resource: obj}
}

// Lookup implements the WorkloadDispatcher contract.
func (d Dispatcher) Lookup(kind string) (WorkloadRenderer, error) {
	r, ok := d.Renderers[kind]
	if !ok {
		return nil, ErrUnknownType
	}

	return r, nil
}
