/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"errors"

	"github.com/project-radius/radius/pkg/algorithm/graph"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/project-radius/radius/pkg/ucp/resources/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	// LocalID is a logical identifier scoped to the owning Radius resource. This is only needed or used
	// when a resource has a dependency relationship. LocalIDs do not have any particular format or meaning
	// beyond being compared to determine dependency relationships.
	LocalID string `json:"localID"`

	// ID is the UCP resource ID of the underlying resource.
	ID resources.ID `json:"id"`

	// RadiusManaged determines whether Radius manages the lifecycle of the underlying resource.
	RadiusManaged *bool `json:"radiusManaged"`

	// CreateResource describes data that will be used to create a resource. This is never saved to the database.
	CreateResource *Resource `json:"-"`
}

// Resource describes data that will be used to create a resource. This data is not saved to the database.
type Resource struct {
	// Data is the arbitrary data that will be passed to the handler.
	Data any
	// ResourceType is the type of resource that will be created. This is used for dispatching to the correct handler.
	ResourceType resourcemodel.ResourceType
	// Dependencies is the set of LocalIDs of the resources that are required to be deployed before this resource can be deployed.
	Dependencies []string
}

// GetResourceType returns the ResourceType of the OutputResource.
func (or OutputResource) GetResourceType() resourcemodel.ResourceType {
	// There are two possible states:
	//
	// 1) The resource already exists in which case we have a resource ID.
	// 2) The resource will be created, in which case we know the resource type, but don't have an ID.
	if or.CreateResource != nil {
		return or.CreateResource.ResourceType
	}

	if or.ID.IsUCPQualfied() && len(or.ID.ScopeSegments()) > 0 {
		return resourcemodel.ResourceType{
			Provider: or.ID.ScopeSegments()[0].Type,
			Type:     or.ID.Type(),
		}
	} else if len(or.ID.ScopeSegments()) > 0 {
		// Legacy ARM case
		return resourcemodel.ResourceType{
			Provider: resourcemodel.ProviderAzure,
			Type:     or.ID.Type(),
		}
	}

	return resourcemodel.ResourceType{}
}

// Key localID of the output resource is used as the key in DependencyItem for output resources.
func (resource OutputResource) Key() string {
	return resource.LocalID
}

// GetDependencies returns a slice of strings containing the LocalIDs of the OutputResource's dependencies, or an error if
// any of the dependencies are missing a LocalID.
func (resource OutputResource) GetDependencies() ([]string, error) {
	if resource.CreateResource == nil {
		return nil, nil
	}

	dependencies := []string{}
	for _, dependency := range resource.CreateResource.Dependencies {
		if dependency == "" {
			return dependencies, errors.New("missing localID for outputresource")
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, nil
}

// IsRadiusManaged checks if the RadiusManaged field of the OutputResource struct is set and returns its value.
func (resource OutputResource) IsRadiusManaged() bool {
	if resource.RadiusManaged == nil {
		return false
	}

	return *resource.RadiusManaged
}

// OrderOutputResources orders the given OutputResources based on their dependencies (i.e. deployment order)
// and returns the ordered OutputResources or an error.
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

// NewKubernetesOutputResource creates an OutputResource object with the given resourceType, localID, obj and objectMeta.
func NewKubernetesOutputResource(localID string, obj runtime.Object, objectMeta metav1.ObjectMeta) OutputResource {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return OutputResource{
		LocalID: localID,
		ID:      resources_kubernetes.IDFromMeta(resources_kubernetes.PlaneNameTODO, gvk, objectMeta),
		CreateResource: &Resource{
			ResourceType: resourcemodel.ResourceType{
				Type:     resources_kubernetes.ResourceTypeFromGVK(gvk),
				Provider: resourcemodel.ProviderKubernetes,
			},
			Data: obj,
		},
	}
}

// GetGCOutputResources [GC stands for Garbage Collection] compares two slices of OutputResource and
// returns a slice of OutputResource that contains the elements that are in the "before" slice but not in the "after".
func GetGCOutputResources(after []OutputResource, before []OutputResource) []OutputResource {
	// We can easily determine which resources have changed via a brute-force search comparing IDs.
	// The lists of resources we work with are small, so this is fine.
	diff := []OutputResource{}
	for _, beforeResource := range before {
		found := false
		for _, afterResource := range after {
			if resources.IDEquals(beforeResource.ID, afterResource.ID) {
				found = true
				break
			}
		}

		if !found {
			diff = append(diff, beforeResource)
		}
	}

	return diff
}
