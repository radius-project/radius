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

package util

// ApplyOutputsMapping maps module output names to resource property names using the provided outputs map.
// Keys in outputsMap are resource property names, values are module output names.
// When outputsMap is nil or empty, the original values and secrets are returned (nil maps are
// normalized to empty maps so callers always receive non-nil maps).
func ApplyOutputsMapping(values map[string]any, secrets map[string]any, outputsMap map[string]string) (map[string]any, map[string]any) {
	if len(outputsMap) == 0 {
		if values == nil {
			values = map[string]any{}
		}
		if secrets == nil {
			secrets = map[string]any{}
		}
		return values, secrets
	}

	mappedValues := make(map[string]any)
	mappedSecrets := make(map[string]any)

	for propertyName, outputName := range outputsMap {
		if val, ok := values[outputName]; ok {
			mappedValues[propertyName] = val
		}
		if val, ok := secrets[outputName]; ok {
			mappedSecrets[propertyName] = val
		}
	}

	return mappedValues, mappedSecrets
}
