// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package processors

import (
	"fmt"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// GetOutputResourcesFromRecipe is a utility function that converts a resource ID provided by a user into an
// OutputResource. This should be used for processing mode == 'resource'.
func GetOutputResourceFromResourceID(resourceID string) (rpv1.OutputResource, error) {
	id, err := resources.ParseResource(resourceID)
	if err != nil {
		return rpv1.OutputResource{}, &ValidationError{Message: fmt.Sprintf("resource id %q is invalid", resourceID)}
	}

	identity := resourcemodel.FromUCPID(id, "")
	result := rpv1.OutputResource{
		LocalID:       fmt.Sprintf("Resource%d", 0), // The dependency sorting code requires unique LocalIDs
		Identity:      identity,
		ResourceType:  *identity.ResourceType,
		RadiusManaged: to.Ptr(false), // Generally when we parse a resource ID from a resource field, it's externally managed.
	}

	return result, nil
}

// GetOutputResourcesFromRecipe is a utility function that converts the resources in the recipe output into a list of OutputResources.
func GetOutputResourcesFromRecipe(output *recipes.RecipeOutput) ([]rpv1.OutputResource, error) {
	results := []rpv1.OutputResource{}
	for i, resource := range output.Resources {
		id, err := resources.ParseResource(resource)
		if err != nil {
			return nil, &ValidationError{Message: fmt.Sprintf("resource id %q returned by recipe is invalid", resource)}
		}

		identity := resourcemodel.FromUCPID(id, "")
		result := rpv1.OutputResource{
			LocalID:       fmt.Sprintf("RecipeResource%d", i), // The dependency sorting code requires unique LocalIDs
			Identity:      identity,
			ResourceType:  *identity.ResourceType,
			RadiusManaged: to.Ptr(true),
		}

		results = append(results, result)
	}

	return results, nil
}
