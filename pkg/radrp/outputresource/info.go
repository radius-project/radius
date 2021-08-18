// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

import (
	"errors"

	"github.com/Azure/radius/pkg/algorithm/graph"
	"github.com/Azure/radius/pkg/health"
)

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	LocalID      string
	HealthID     string
	Type         string
	Kind         string
	Deployed     bool
	Managed      bool
	Info         interface{}
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

// ARMInfo info required to identify an ARM resource
type ARMInfo struct {
	ID           string `bson:"id"`
	ResourceType string `bson:"resourceType"`
	APIVersion   string `bson:"apiVersion"`
}

// K8sInfo info required to identify a Kubernetes resource
type K8sInfo struct {
	Kind       string `bson:"kind"`
	APIVersion string `bson:"apiVersion"`
	Name       string `bson:"name"`
	Namespace  string `bson:"namespace"`
}

// AADPodIdentity pod identity for AKS cluster to enable access to keyvault
type AADPodIdentity struct {
	AKSClusterName string `bson:"aksClusterName"`
	Name           string `bson:"name"`
	Namespace      string `bson:"namespace"`
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

// GetResourceID returns the identifier of the entity/resource to be queried by the health service
func (resource OutputResource) GetResourceID() string {
	if resource.Info == nil {
		return ""
	}

	if resource.Type == TypeARM {
		return resource.Info.(ARMInfo).ID
	} else if resource.Type == TypeAADPodIdentity {
		return resource.Info.(AADPodIdentity).AKSClusterName + "-" + resource.Info.(AADPodIdentity).Name
	} else if resource.Type == TypeKubernetes {
		return resource.Info.(K8sInfo).Namespace + "-" + resource.Info.(K8sInfo).Name
	}
	return ""
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

func NewOutputResource(
	resource interface{},
	deployed bool,
	localID string,
	managed bool,
	resourceKind string,
	outputResourceType string,
	outputResourceInfo interface{},
	healthProbe health.Monitor,
	healthOpts ...health.HealthCheckOption,
) OutputResource {
	or := OutputResource{
		Resource: resource,
		Deployed: deployed,
		LocalID:  localID,
		Managed:  managed,
		Kind:     resourceKind,
		Type:     outputResourceType,
		Info:     outputResourceInfo,
	}

	return or
}
