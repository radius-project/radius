// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

import (
	"errors"

	"github.com/project-radius/radius/pkg/algorithm/graph"
	"github.com/project-radius/radius/pkg/resourcemodel"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	// LocalID is a logical identifier scoped to the owning Radius resource.
	LocalID string `json:"localID"`

	// Identity uniquely identifies the underlying resource within its platform..
	Identity resourcemodel.ResourceIdentity `json:"identity"`

	// Resource type specifies the 'provider' and 'kind' used to look up the resource handler for processing
	ResourceType resourcemodel.ResourceType `json:"resourceType"`

	// Resource type defined by the provider for this resource. Example for Azure, a resource type is of the format: Microsoft.DocumentDB/databaseAccounts
	ProviderResourceType string `json:"providerResourceType"`

	RadiusManaged *bool                `json:"radiusManaged"`
	Deployed      bool                 `json:"deployed"`
	Resource      any                  `json:"resource,omitempty"`
	Dependencies  []Dependency         // resources that are required to be deployed before this resource can be deployed - used for parent/child resources.
	Status        OutputResourceStatus `json:"status,omitempty"`
}

type Dependency struct {
	// LocalID is the LocalID of the dependency.
	LocalID string
}

// OutputResourceStatus represents the status of the Output Resource
type OutputResourceStatus struct {
	ProvisioningState        string `json:"provisioningState"`
	ProvisioningErrorDetails string `json:"provisioningErrorDetails"`
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

func (resource OutputResource) IsRadiusManaged() bool {
	if resource.RadiusManaged == nil {
		return false
	}

	return *resource.RadiusManaged
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
		Provider: resourcemodel.ProviderKubernetes,
	}
	return OutputResource{
		LocalID:      localID,
		Deployed:     false,
		ResourceType: rt,
		Identity:     resourcemodel.NewKubernetesIdentity(&rt, obj, objectMeta),
		Resource:     obj,
		Dependencies: []Dependency{},
	}
}

// GetGCOutputResources [GC stands for Garbage Collection] returns the elements
// that are in the oldResource but not in the updatedResource
func GetGCOutputResources(after []OutputResource, before []OutputResource) []OutputResource {
	afterMap := map[string][]OutputResource{}

	for _, outputResource := range after {
		id := outputResource.LocalID
		orArr := []OutputResource{}

		if arr, ok := afterMap[id]; ok {
			orArr = arr
		}

		orArr = append(orArr, outputResource)
		afterMap[id] = orArr
	}

	diff := []OutputResource{}
	for _, outputResource := range before {
		id := outputResource.LocalID

		// If there is a resource or a group of resources in before(old) outputResources
		// array with a LocalID that is not in the after(new) outputResources array, then
		// we have to to delete those resources.
		if _, found := afterMap[id]; !found {
			diff = append(diff, outputResource)
			continue
		}

		// Otherwise we have to check each resource for their equivalence on ResourceType.Type
		// and ResourceType.Provider properties. If there is no match, we have to delete that
		// resource. Meaning that new outputResources doesn't have that resource in the old array.
		found := false
		for _, innerOutputResource := range afterMap[id] {
			if outputResource.ResourceType.Type == innerOutputResource.ResourceType.Type &&
				outputResource.ResourceType.Provider == innerOutputResource.ResourceType.Provider {
				found = true
				break
			}
		}

		if !found {
			diff = append(diff, outputResource)
		}
	}

	return diff
}
