// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloads

import (
	"context"
	"errors"

	"github.com/Azure/radius/pkg/radrp/components"
)

// ErrUnknownType is the error reported when the workload type is unknown or unsupported.
var ErrUnknownType = errors.New("workload type is unsupported")

// InstantiatedWorkload workload provides all of the information needed to render a workload.
type InstantiatedWorkload struct {
	Application   string
	Name          string
	Workload      components.GenericComponent
	BindingValues map[components.BindingKey]components.BindingState
	Namespace     string
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
