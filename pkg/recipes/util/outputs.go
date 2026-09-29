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
	// MissingMappings contains the deterministically ordered invalid mapping descriptions.
	MissingMappings []string
	// AvailableOutputs contains the deterministically ordered module output names.
	AvailableOutputs []string
}

// MissingOutputValuesError reports secret mappings whose deployment output is missing or null.
type MissingOutputValuesError struct {
	// MissingMappings contains the deterministically ordered mappings without runtime values.
	MissingMappings []string
	// AvailableOutputs contains the deterministically ordered non-null deployment output names.
	AvailableOutputs []string
}

// Error describes invalid mappings and lists the module's declared outputs.
func (e *OutputMappingError) Error() string {
	return fmt.Sprintf(
		"invalid outputs mapping: no declared module output matches %s; available module outputs: %s",
		strings.Join(e.MissingMappings, ", "),
		formatOutputNames(e.AvailableOutputs))
}

// Error describes missing secret values and lists the non-null deployment outputs.
func (e *MissingOutputValuesError) Error() string {
	return fmt.Sprintf(
		"invalid outputs mapping: missing deployment output values for %s; available deployment outputs: %s",
		strings.Join(e.MissingMappings, ", "),
		formatOutputNames(e.AvailableOutputs))
}

// ValidateOutputsMapping checks every mapping against the module's declared outputs.
func ValidateOutputsMapping(recipeName string, resourceType string, declaredOutputs []string, outputsMap map[string]string, secretOutputsMap map[string]string) error {
	declared := make(map[string]struct{}, len(declaredOutputs))
	for _, outputName := range declaredOutputs {
		declared[outputName] = struct{}{}
	}

	missingMappings := findMissingMappings("outputs", declared, outputsMap)
	missingMappings = append(missingMappings, findMissingMappings("secrets", declared, secretOutputsMap)...)

	if len(missingMappings) == 0 {
		return nil
	}

	availableOutputs := make([]string, 0, len(declared))
	for outputName := range declared {
		availableOutputs = append(availableOutputs, outputName)
	}
	sort.Strings(missingMappings)
	sort.Strings(availableOutputs)

	return fmt.Errorf("recipe %q for resource type %q: %w", recipeName, resourceType, &OutputMappingError{
		MissingMappings:  missingMappings,
		AvailableOutputs: availableOutputs,
	})
}

func findMissingMappings(label string, declared map[string]struct{}, mappings map[string]string) []string {
	missingMappings := []string{}
	for propertyName, outputName := range mappings {
		if _, ok := declared[outputName]; !ok {
			missingMappings = append(missingMappings, formatMapping(label, propertyName, outputName))
		}
	}
	return missingMappings
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
// to empty maps so callers always receive non-nil maps). Missing or null ordinary outputs are omitted.
// Missing or null secret outputs return MissingOutputValuesError because silently dropping them could clear
// previously materialized secret data.
func ApplyOutputsMapping(values map[string]any, secrets map[string]any, outputsMap map[string]string, secretOutputsMap map[string]string) (map[string]any, map[string]any, error) {
	if len(outputsMap) == 0 && len(secretOutputsMap) == 0 {
		if values == nil {
			values = map[string]any{}
		}
		if secrets == nil {
			secrets = map[string]any{}
		}
		return values, secrets, nil
	}

	mappedValues := make(map[string]any)
	mappedSecrets := make(map[string]any)

	// outputsMap: preserve the module's own value/secret classification.
	for propertyName, outputName := range outputsMap {
		if val, ok := nonNullOutput(values, outputName); ok {
			mappedValues[propertyName] = val
		}
		if val, ok := nonNullOutput(secrets, outputName); ok {
			mappedSecrets[propertyName] = val
		}
	}

	// secretOutputsMap: always emit as a secret, whether the module declared the output sensitive or not.
	missingSecretMappings := []string{}
	for propertyName, outputName := range secretOutputsMap {
		if val, ok := nonNullOutput(secrets, outputName); ok {
			mappedSecrets[propertyName] = val
		} else if val, ok := nonNullOutput(values, outputName); ok {
			mappedSecrets[propertyName] = val
		} else {
			missingSecretMappings = append(missingSecretMappings, formatMapping("secrets", propertyName, outputName))
		}
	}

	if len(missingSecretMappings) > 0 {
		sort.Strings(missingSecretMappings)
		return nil, nil, &MissingOutputValuesError{
			MissingMappings:  missingSecretMappings,
			AvailableOutputs: availableOutputNames(values, secrets),
		}
	}

	return mappedValues, mappedSecrets, nil
}

func nonNullOutput(outputs map[string]any, outputName string) (any, bool) {
	value, ok := outputs[outputName]
	return value, ok && value != nil
}

func availableOutputNames(values map[string]any, secrets map[string]any) []string {
	available := make(map[string]struct{}, len(values)+len(secrets))
	for outputName, value := range values {
		if value != nil {
			available[outputName] = struct{}{}
		}
	}
	for outputName, value := range secrets {
		if value != nil {
			available[outputName] = struct{}{}
		}
	}

	outputNames := make([]string, 0, len(available))
	for outputName := range available {
		outputNames = append(outputNames, outputName)
	}
	sort.Strings(outputNames)
	return outputNames
}

func formatMapping(label string, propertyName string, outputName string) string {
	return fmt.Sprintf("%s[%q] -> %q", label, propertyName, outputName)
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
