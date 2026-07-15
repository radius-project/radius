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

// ApplyOutputsMapping renames a direct module's outputs onto resource property names.
//
// Keys in outputsMap and secretOutputsMap are resource property names; values are module output names.
//   - outputsMap entries route a module output to a value or a secret depending on how the module itself
//     classified it (a secure Bicep output / sensitive Terraform output lands in secrets, otherwise values).
//   - secretOutputsMap entries always route the referenced module output to secrets, regardless of how the
//     module classified it. This lets a recipe pack force an output (for example an AVM module's
//     `primaryConnectionString`, which the module declares as a plain string) to be treated as a secret.
//
// When both maps are empty, the original values and secrets are returned unchanged (nil maps are normalized
// to empty maps so callers always receive non-nil maps).
func ApplyOutputsMapping(values map[string]any, secrets map[string]any, outputsMap map[string]string, secretOutputsMap map[string]string) (map[string]any, map[string]any) {
	if len(outputsMap) == 0 && len(secretOutputsMap) == 0 {
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

	// outputsMap: preserve the module's own value/secret classification.
	for propertyName, outputName := range outputsMap {
		if val, ok := values[outputName]; ok {
			mappedValues[propertyName] = val
		}
		if val, ok := secrets[outputName]; ok {
			mappedSecrets[propertyName] = val
		}
	}

	// secretOutputsMap: always emit as a secret, whether the module declared the output sensitive or not.
	for propertyName, outputName := range secretOutputsMap {
		if val, ok := secrets[outputName]; ok {
			mappedSecrets[propertyName] = val
		} else if val, ok := values[outputName]; ok {
			mappedSecrets[propertyName] = val
		}
	}

	return mappedValues, mappedSecrets
}
