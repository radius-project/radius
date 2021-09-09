// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloadsv1alpha3

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/model/resourcesv1alpha3"
	"github.com/Azure/radius/pkg/radrp/outputresource"
)

// InstantiatedWorkload workload provides all of the information needed to render a workload.
type InstantiatedWorkload struct {
	Application string
	Name        string
	Workload    resourcesv1alpha3.GenericResource
	// TODO binding values should instead be resources
	// DependsOn map[string]resourcesv1alpha3.GenericResource
	Namespace string
}

// WorkloadRenderer defines the interface for rendering a Kubernetes workload definition
// into a set of raw Kubernetes resources.
//
// The idea is that this represents *fan-out* in terms of the implementation. All of the APIs here
// could be replaced with REST calls.
//go:generate mockgen -destination=../../pkg/renderers/mock_renderer.go -package=renderers github.com/Azure/radius/pkg/workloads WorkloadRenderer
type WorkloadRenderer interface {
	// Render is called for the component to provide its output resources.
	Render(ctx context.Context, workload InstantiatedWorkload) ([]outputresource.OutputResource, error)

	// Get dependencies returns a list of resource ids to track
	// GetDependencies(ctx context.Context, workload InstantiatedWorkload) ([]string, error)
}

// WorkloadResourceProperties represents the properties output by deploying a resource.
type WorkloadResourceProperties struct {
	Type       string
	LocalID    string
	Properties map[string]string
}

// FindByLocalID finds a WorkloadResourceProperties with a matching LocalID. Returns an error if not found.
func FindByLocalID(resources []WorkloadResourceProperties, localID string) (*WorkloadResourceProperties, error) {
	for _, resource := range resources {
		if resource.LocalID == localID {
			return &resource, nil
		}
	}

	names := []string{}
	for _, resource := range resources {
		names = append(names, resource.LocalID)
	}

	return nil, fmt.Errorf("cannot find a resource matching id %s searched %d resources: %s", localID, len(resources), strings.Join(names, " "))
}
