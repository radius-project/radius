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

import (
	"fmt"
	"sort"
	"strings"
)

// OutputMappingError reports mappings that refer to undeclared module outputs.
type OutputMappingError struct {
	missingMappings  []string
	availableOutputs []string
}

// Error describes invalid mappings and lists the module's declared outputs.
func (e *OutputMappingError) Error() string {
	return fmt.Sprintf(
		"invalid outputs mapping: no declared module output matches %s; available module outputs: %s",
		strings.Join(e.missingMappings, ", "),
		formatOutputNames(e.availableOutputs))
}

// ValidateOutputsMapping checks every mapping against the module's declared outputs.
func ValidateOutputsMapping(declaredOutputs []string, outputsMap map[string]string, secretOutputsMap map[string]string) error {
	declared := make(map[string]struct{}, len(declaredOutputs))
	for _, outputName := range declaredOutputs {
		declared[outputName] = struct{}{}
	}

	missingMappings := []string{}
	for propertyName, outputName := range outputsMap {
		if _, ok := declared[outputName]; !ok {
			missingMappings = append(missingMappings, fmt.Sprintf("%q -> %q", propertyName, outputName))
		}
	}

	for propertyName, outputName := range secretOutputsMap {
		if _, ok := declared[outputName]; !ok {
			missingMappings = append(missingMappings, fmt.Sprintf("%q -> %q", "secrets."+propertyName, outputName))
		}
	}

	if len(missingMappings) == 0 {
		return nil
	}

	availableOutputs := make([]string, 0, len(declared))
	for outputName := range declared {
		availableOutputs = append(availableOutputs, outputName)
	}
	sort.Strings(missingMappings)
	sort.Strings(availableOutputs)

	return &OutputMappingError{
		missingMappings:  missingMappings,
		availableOutputs: availableOutputs,
	}
}

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

func formatOutputNames(outputNames []string) string {
	if len(outputNames) == 0 {
		return "none"
	}

	quotedOutputs := make([]string, len(outputNames))
	for i, outputName := range outputNames {
		quotedOutputs[i] = fmt.Sprintf("%q", outputName)
	}
	return strings.Join(quotedOutputs, ", ")
}
