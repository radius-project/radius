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

package controller

import (
	"encoding/json"
	"fmt"
)

const (
	errMarshalResource             = "failed to marshal resource"
	errUnmarshalResourceProperties = "failed to unmarshal resource for properties"
)

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
