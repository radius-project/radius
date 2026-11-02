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
	"maps"
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

// FlattenProperties takes a string-keyed map, which may contain nested objects, and recursively flattens it into a single-level map.
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

// UnflattenProperties takes in a map of strings to any type and returns an object with the nested
// properties unflattened.
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
			for i := range splitPath {
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

// GeneratePatch takes in the current state, desired state and type schema of a resource and returns a JSON patch to update the
// current state to the desired state, taking into account read-only, create-only and conditional-create-only properties.
//
// A value AWS reports in a different form than the template set it counts as unchanged, using the schema's rules: an
// array the schema marks "insertionOrder": false is compared ignoring order, and a property with a supported
// "propertyTransform" is compared through it. An empty object AWS reports is kept unless the template sets values
// inside it.
func GeneratePatch(currentState []byte, desiredState []byte, schema []byte) (jsondiff.Patch, error) {
	// See: https://github.com/radius-project/radius/blob/main/docs/adr/ucp/001-aws-resource-updating.md

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
	// FlattenProperties drops an empty object, so a property AWS reports as {}, such as DataProtectionPolicy on a log
	// group, would look removed and every redeploy would send a remove. Keep it as AWS reports it.
	emptyObjectPaths := emptyObjects(currentStateObject, "")
	for path := range emptyObjectPaths {
		flattenedCurrentStateObject[path] = map[string]any{}
	}

	// Get the desired state of the resource
	var desiredStateObject map[string]any
	err = json.Unmarshal(desiredState, &desiredStateObject)
	if err != nil {
		return nil, err
	}
	flattenedDesiredStateObject := FlattenProperties(desiredStateObject)

	rules := parseSchemaRules(schema)

	// A value AWS reports in a different form, such as reordered subnets or a fuller engine version, is unchanged:
	// use AWS's value so the patch leaves it alone.
	for k, v := range flattenedDesiredStateObject {
		if current, ok := flattenedCurrentStateObject[k]; ok && rules.equivalentAt(k, v, current, desiredStateObject) {
			flattenedDesiredStateObject[k] = current
		}
	}

	// Add read-only and create-only properties from current state to the desired state
	for k, v := range flattenedCurrentStateObject {
		if emptyObjectPaths[k] && hasPathUnder(flattenedDesiredStateObject, k) {
			// The template sets values inside an object AWS reports as empty; copying {} would overwrite them.
			continue
		}

		property := fmt.Sprintf("/properties/%s", k)

		isCreateOnlyProperty := slices.Contains(resourceTypeSchema.CreateOnlyProperties, property)
		isWriteOnlyProperty := slices.Contains(resourceTypeSchema.WriteOnlyProperties, property)

		// If the property is create-only and write-only, then upsert it to the desired state.
		// this will cause a no-op in the patch since it will exactly match the current state
		if isWriteOnlyProperty && isCreateOnlyProperty {
			flattenedDesiredStateObject[k] = v
		} else if _, exists := flattenedDesiredStateObject[k]; !exists {
			// Preserve all properties from the current state that the user didn't
			// explicitly specify. This prevents generating "remove" operations for
			// properties that AWS may have added since the template was written,
			// and ensures the patch only modifies properties the user explicitly
			// specified. This matches the standard behavior of infrastructure-as-code
			// tools like Terraform and CloudFormation, where unmentioned properties
			// are left unchanged.
			flattenedDesiredStateObject[k] = v
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

// emptyObjects returns the flattened paths of the empty objects in state, which FlattenProperties leaves out.
func emptyObjects(state map[string]any, prefix string) map[string]bool {
	out := map[string]bool{}
	for k, v := range state {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if len(m) == 0 {
			out[prefix+k] = true
			continue
		}
		maps.Copy(out, emptyObjects(m, prefix+k+"/"))
	}
	return out
}

// hasPathUnder reports whether flattened has a key inside the object at path.
func hasPathUnder(flattened map[string]any, path string) bool {
	for k := range flattened {
		if strings.HasPrefix(k, path+"/") {
			return true
		}
	}
	return false
}

// ParsePropertyName transforms a propertyIdentifer of the form /properties/<propertyName> to <propertyName>.
// It returns an error if the property identifier is not in the expected format.
func ParsePropertyName(propertyIdentifier string) (string, error) {
	prefix := "/properties/"
	if after, ok := strings.CutPrefix(propertyIdentifier, prefix); ok {
		return after, nil
	}
	return "", fmt.Errorf("property identifier %s is not in the format /properties/<propertyName>", propertyIdentifier)
}
