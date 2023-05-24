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

package operations

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/wI2L/jsondiff"
	"golang.org/x/exp/slices"
)

type ResourceTypeSchema struct {
	Properties                      map[string]any `json:"properties,omitempty"`
	ReadOnlyProperties              []string       `json:"readOnlyProperties,omitempty"`
	CreateOnlyProperties            []string       `json:"createOnlyProperties,omitempty"`
	ConditionalCreateOnlyProperties []string       `json:"conditionalCreateOnlyProperties,omitempty"`
	WriteOnlyProperties             []string       `json:"writeOnlyProperties,omitempty"`
}

// FlattenProperties flattens a state object.
// For example:
//
//	"NumShards": 1
//	"ClusterEndpoint": {
//	  "Address": "test-address"
//	  "Port": 3000
//	}
//
// Gets transformed to:
//
//	"NumShards": 1
//	"ClusterEndpoint/Address": "test-address"
//	"ClusterEndpoint/Port": 3000
func FlattenProperties(state map[string]any) map[string]any {
	flattenedState := map[string]any{}

	for k, v := range state {
		// If the value is a map, flatten it
		if reflect.TypeOf(v).Kind() == reflect.Map {
			flattenedSubState := FlattenProperties(v.(map[string]any))

			for subK, subV := range flattenedSubState {
				key := k + "/" + subK
				flattenedState[key] = subV
			}
		} else {
			flattenedState[k] = v
		}
	}

	return flattenedState
}

// UnflattenProperties unflattens a flattened state object.
// For example:
//
//	"NumShards": 1
//	"ClusterEndpoint/Address": "test-address"
//	"ClusterEndpoint/Port": 3000
//
// Gets transformed to:
//
//	"NumShards": 1
//	"ClusterEndpoint": {
//	  "Address": "test-address"
//	  "Port": 3000
//	}
func UnflattenProperties(state map[string]any) map[string]any {
	unflattenedState := map[string]any{}

	for k, v := range state {
		splitPath := strings.Split(k, "/")
		rootKey := splitPath[0]

		if len(splitPath) == 1 {
			unflattenedState[rootKey] = v
		} else {
			var currentState any = unflattenedState
			for i := 0; i < len(splitPath); i++ {
				subKey := splitPath[i]
				if i == len(splitPath)-1 {
					if currentStateMap, ok := currentState.(map[string]any); ok {
						currentStateMap[subKey] = v
					}
				} else {
					if currentStateMap, ok := currentState.(map[string]any); ok {
						if _, exists := currentStateMap[subKey]; !exists {
							currentStateMap[subKey] = map[string]any{}
						}

						currentState = currentStateMap[subKey]
					}
				}
			}
		}
	}

	return unflattenedState
}

// GeneratePatch generates a JSON patch based on a given current state, desired state, and resource type schema
func GeneratePatch(currentState []byte, desiredState []byte, schema []byte) (jsondiff.Patch, error) {
	// See: https://github.com/project-radius/radius/blob/main/docs/adr/ucp/001-aws-resource-updating.md

	// Get the resource type schema - this will tell us the properties of the
	// resource as well as which properties are read-only, create-only, etc.
	var resourceTypeSchema ResourceTypeSchema
	err := json.Unmarshal(schema, &resourceTypeSchema)
	if err != nil {
		return nil, err
	}

	// Get the current state of the resource
	var currentStateObject map[string]any
	err = json.Unmarshal(currentState, &currentStateObject)
	if err != nil {
		return nil, err
	}
	flattenedCurrentStateObject := FlattenProperties(currentStateObject)

	// Get the desired state of the resource
	var desiredStateObject map[string]any
	err = json.Unmarshal(desiredState, &desiredStateObject)
	if err != nil {
		return nil, err
	}
	flattenedDesiredStateObject := FlattenProperties(desiredStateObject)

	// Add read-only and create-only properties from current state to the desired state
	for k, v := range flattenedCurrentStateObject {
		property := fmt.Sprintf("/properties/%s", k)

		isCreateOnlyProperty := slices.Contains(resourceTypeSchema.CreateOnlyProperties, property)
		isWriteOnlyProperty := slices.Contains(resourceTypeSchema.WriteOnlyProperties, property)

		// If the property is create-only and write-only, then upsert it to the desired state.
		// this will cause a no-op in the patch since it will exactly match the current state
		if isWriteOnlyProperty && isCreateOnlyProperty {
			flattenedDesiredStateObject[k] = v
		} else if _, exists := flattenedDesiredStateObject[k]; !exists {
			// Add the property (if not exists already) to the desired state if it is a read-only, create-only,
			// or conditional-create-only property. This ensures that these types of properties result in a
			// no-op in the patch if they aren't updated in the desired state
			isReadOnlyProperty := slices.Contains(resourceTypeSchema.ReadOnlyProperties, property)
			isConditionalCreateOnlyProperty := slices.Contains(resourceTypeSchema.ConditionalCreateOnlyProperties, property)
			if isReadOnlyProperty || isCreateOnlyProperty || isConditionalCreateOnlyProperty {
				flattenedDesiredStateObject[k] = v
			}
		}
	}

	// Convert desired patch state back into unflattened object
	unflattenedDesiredStateObject := UnflattenProperties(flattenedDesiredStateObject)

	// Marshal desired state into bytes
	updatedDesiredState, err := json.Marshal(unflattenedDesiredStateObject)
	if err != nil {
		return nil, err
	}

	// Calculate the patch based on the current state and the updated desired state
	return jsondiff.CompareJSON(currentState, updatedDesiredState)
}

// ParsePropertyName transforms a propertyIdentifer of the form /properties/<propertyName> to <propertyName>
func ParsePropertyName(propertyIdentifier string) (string, error) {
	prefix := "/properties/"
	if strings.HasPrefix(propertyIdentifier, prefix) {
		return strings.TrimPrefix(propertyIdentifier, prefix), nil
	}
	return "", fmt.Errorf("property identifier %s is not in the format /properties/<propertyName>", propertyIdentifier)
}
