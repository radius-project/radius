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

package processors

import (
	"fmt"

	"github.com/project-radius/radius/pkg/portableresources"
	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// GetOutputResourcesFromResourcesField parses a slice of resource references and converts each resource into an OutputResource.
// It returns a slice of output resources and an error if any of the resource references are invalid. This should be used for
// processing the '.properties.resources' field of a resource.
func GetOutputResourcesFromResourcesField(field []*portableresources.ResourceReference) ([]rpv1.OutputResource, error) {
	results := []rpv1.OutputResource{}
	for _, resource := range field {
		id, err := resources.ParseResource(resource.ID)
		if err != nil {
			return nil, &ValidationError{Message: fmt.Sprintf("resource id %q is invalid", resource.ID)}
		}

		result := rpv1.OutputResource{
			ID:            id,
			RadiusManaged: to.Ptr(false), // Generally when we parse a resource ID from a resource field, it's externally managed.
		}
		results = append(results, result)
	}

	return results, nil
}

// GetOutputResourcesFromRecipe parses the output resources from a recipe and returns a slice of OutputResource objects,
// returning an error if any of the resources are invalid.
func GetOutputResourcesFromRecipe(output *recipes.RecipeOutput) ([]rpv1.OutputResource, error) {
	results := []rpv1.OutputResource{}
	for _, resource := range output.Resources {
		id, err := resources.ParseResource(resource)
		if err != nil {
			return nil, &ValidationError{Message: fmt.Sprintf("resource id %q returned by recipe is invalid", resource)}
		}

		result := rpv1.OutputResource{
			ID:            id,
			RadiusManaged: to.Ptr(true),
		}

		results = append(results, result)
	}

	return results, nil
}
