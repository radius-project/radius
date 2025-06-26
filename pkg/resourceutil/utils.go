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

package resourceutil

import (
	"encoding/json"
	"fmt"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	errMarshalResource             = "failed to marshal resource"
	errUnmarshalResourceProperties = "failed to unmarshal resource for properties"
)

// BasicProperties is a list of common properties that are expected to be present in all resources
var BasicProperties = []string{"application", "environment", "status", "connections"}

// GetPropertiesFromResource extracts the "properties" field from the resource
// by serializing it to JSON and deserializing just the "properties" field.
func GetPropertiesFromResource[P any](resource P) (map[string]any, error) {
	// Serialize the resource to JSON
	bytes, err := json.Marshal(resource)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errMarshalResource, err)
	}

	// Define a minimal struct to capture just the "properties" field
	var partialResource struct {
		Properties map[string]any `json:"properties"`
	}

	// Deserialize the JSON into the partialResource struct
	if err := json.Unmarshal(bytes, &partialResource); err != nil {
		return nil, fmt.Errorf("%s: %w", errUnmarshalResourceProperties, err)
	}

	// Return an empty map if properties is nil
	if partialResource.Properties == nil {
		return map[string]any{}, nil
	}

	return partialResource.Properties, nil
}

// GetConnectionNameandSourceIDs extracts the connected resource IDs from the resource's properties.
// It returns a map where the keys are connection names and the values are the corresponding connected resource's IDs.
func GetConnectionNameandSourceIDs[P any](resource P) (map[string]string, error) {
	connectionNamesAndSourceIDs := map[string]string{}
	resourceProperties, err := GetPropertiesFromResource(resource)
	if err != nil {
		return nil, err
	}

	if resourceProperties != nil && resourceProperties["connections"] != nil {
		connections, ok := resourceProperties["connections"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("failed to get connections from resource properties: %w", err)
		}

		for connectionName, connectionProperties := range connections {
			if source, ok := connectionProperties.(map[string]any)["source"]; ok {
				if resourceID, ok := source.(string); ok {
					_, err := resources.Parse(resourceID) // Validate the resource ID format
					if err != nil {
						return nil, fmt.Errorf("invalid resource ID in connection %s: %w", connectionName, err)
					}
					connectionNamesAndSourceIDs[connectionName] = resourceID
				} else {
					return nil, fmt.Errorf("source in connection %s is not a string: %w", connectionName, err)
				}
			} else {
				return nil, fmt.Errorf("source not found in connection %q: %w", connectionName, err)
			}
		}
	}

	return connectionNamesAndSourceIDs, nil
}
