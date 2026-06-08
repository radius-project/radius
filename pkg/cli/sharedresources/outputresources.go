/*
Copyright 2026 The Radius Authors.

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

package sharedresources

import (
	"context"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// SharedReference describes another Radius resource that references the same output resource.
type SharedReference struct {
	ResourceID       string
	OutputResourceID string
}

// FindSharedReferences scans Radius resources for output resources that match target's output resources.
func FindSharedReferences(ctx context.Context, client clients.ApplicationsManagementClient, target generated.GenericResource, excludedResourceIDs map[string]bool) ([]SharedReference, error) {
	targetOutputResources := OutputResourcesFromGenericResource(target)
	if len(targetOutputResources) == 0 {
		return nil, nil
	}

	resourceTypes, err := client.ListAllResourceTypesNames(ctx, "local")
	if err != nil {
		return nil, err
	}

	shared := []SharedReference{}
	for _, resourceType := range resourceTypes {
		resources, err := client.ListResourcesOfType(ctx, resourceType)
		if err != nil {
			return nil, err
		}

		for _, candidate := range resources {
			candidateID := stringValue(candidate.ID)
			if candidateID == "" || excludedResourceIDs[candidateID] {
				continue
			}

			for _, targetOutputResource := range targetOutputResources {
				for _, candidateOutputResource := range OutputResourcesFromGenericResource(candidate) {
					if rpv1.OutputResourceMatches(targetOutputResource, candidateOutputResource) {
						shared = append(shared, SharedReference{
							ResourceID:       candidateID,
							OutputResourceID: targetOutputResource.ID.String(),
						})
					}
				}
			}
		}
	}

	return shared, nil
}

// OutputResourcesFromGenericResource extracts output resources from the weakly-typed API resource shape.
func OutputResourcesFromGenericResource(resource generated.GenericResource) []rpv1.OutputResource {
	statusRaw, ok := resource.Properties["status"]
	if !ok || statusRaw == nil {
		return nil
	}

	status, ok := statusRaw.(map[string]any)
	if !ok {
		return nil
	}

	outputResourcesRaw, ok := status["outputResources"]
	if !ok || outputResourcesRaw == nil {
		return nil
	}

	outputResources, ok := outputResourcesRaw.([]any)
	if !ok {
		return nil
	}

	result := []rpv1.OutputResource{}
	for _, outputResourceRaw := range outputResources {
		outputResource, ok := outputResourceRaw.(map[string]any)
		if !ok {
			continue
		}

		id, ok := outputResource["id"].(string)
		if !ok || id == "" {
			continue
		}

		parsedID, err := resources.Parse(id)
		if err != nil {
			continue
		}

		result = append(result, rpv1.OutputResource{
			ID:                   parsedID,
			AdditionalProperties: stringMap(outputResource["additionalProperties"]),
		})
	}

	return result
}

func stringMap(value any) map[string]string {
	if value == nil {
		return nil
	}

	switch typed := value.(type) {
	case map[string]string:
		return typed
	case map[string]any:
		result := map[string]string{}
		for key, value := range typed {
			if stringValue, ok := value.(string); ok {
				result[key] = stringValue
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	default:
		return nil
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
