// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

import (
	"errors"

	"github.com/project-radius/radius/pkg/algorithm/graph"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/resourcemodel"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	// LocalID is a logical identifier scoped to the owning Radius resource.
	LocalID string

	// Identity uniquely identifies the underlying resource within its platform..
	Identity resourcemodel.ResourceIdentity

	// Resource type specifies the 'provider' and 'kind' used to look up the resource handler for processing
	ResourceType resourcemodel.ResourceType

	Deployed     bool
	Resource     interface{}
	Dependencies []Dependency // resources that are required to be deployed before this resource can be deployed
	Status       OutputResourceStatus
}

type Dependency struct {
	// LocalID is the LocalID of the dependency.
	LocalID string
	// Placeholder is a slice of optional placeholder values that can copy values from the dependency.
	Placeholder []Placeholder
}

// OutputResourceStatus represents the status of the Output Resource
type OutputResourceStatus struct {
	ProvisioningState        string    `bson:"provisioningState"`
	ProvisioningErrorDetails string    `bson:"provisioningErrorDetails"`
	HealthState              string    `bson:"healthState"`
	HealthErrorDetails       string    `bson:"healthErrorDetails"`
	Replicas                 []Replica `bson:"replicas,omitempty" structs:"-"` // Ignore stateful property during serialization
}

// Replica represents an individual instance of a resource (Azure/K8s)
type Replica struct {
	ID     string
	Status ReplicaStatus `bson:"status"`
}

// ReplicaStatus represents the status of a replica
type ReplicaStatus struct {
	ProvisioningState string `bson:"provisioningState"`
	HealthState       string `bson:"healthState"`
}

// Key localID of the output resource is used as the key in DependencyItem for output resources.
func (resource OutputResource) Key() string {
	return resource.LocalID
}

// GetDependencies returns list of localId of output resources the resource depends on.
func (resource OutputResource) GetDependencies() ([]string, error) {
	dependencies := []string{}
	for _, dependency := range resource.Dependencies {
		if dependency.LocalID == "" {
			return dependencies, errors.New("missing localID for outputresource")
		}
		dependencies = append(dependencies, dependency.LocalID)
	}
	return dependencies, nil
}

// OrderOutputResources returns output resources ordered based on deployment order
func OrderOutputResources(outputResources []OutputResource) ([]OutputResource, error) {
	unorderedItems := []graph.DependencyItem{}
	for _, outputResource := range outputResources {
		unorderedItems = append(unorderedItems, outputResource)
	}

	dependencyGraph, err := graph.ComputeDependencyGraph(unorderedItems)
	if err != nil {
		return nil, err
	}

	orderedItems, err := dependencyGraph.Order()
	if err != nil {
		return nil, err
	}

	orderedOutput := []OutputResource{}
	for _, item := range orderedItems {
		orderedOutput = append(orderedOutput, item.(OutputResource))
	}

	return orderedOutput, nil
}

func NewKubernetesOutputResource(resourceType string, localID string, obj runtime.Object, objectMeta metav1.ObjectMeta) OutputResource {
	rt := resourcemodel.ResourceType{
		Type:     resourceType,
		Provider: providers.ProviderKubernetes,
	}
	return OutputResource{
		LocalID:      localID,
		Deployed:     false,
		ResourceType: rt,
		Identity:     resourcemodel.NewKubernetesIdentity(&rt, obj, objectMeta),
		Resource:     obj,
	}
}
