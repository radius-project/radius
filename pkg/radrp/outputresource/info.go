// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

import (
	"fmt"

	"github.com/Azure/radius/pkg/algorithm/graph"
)

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	LocalID      string
	Type         string
	Kind         string
	Deployed     bool
	Managed      bool
	Info         interface{}
	Resource     interface{}
	Dependencies []OutputResource // resources that are required to be deployed before this resource can be deployed
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
			return dependencies, fmt.Errorf("missing localID for outputresource kind: %s", dependency.Kind)
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
