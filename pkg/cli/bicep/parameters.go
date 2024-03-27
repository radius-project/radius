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

package bicep

import "fmt"

// ExtractParameters extracts the parameters from the deployment template.
func ExtractParameters(template map[string]any) (map[string]any, error) {
	if template["parameters"] == nil {
		return map[string]any{}, nil
	}

	params, ok := template["parameters"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid template: parameters must be a map of maps, got: %T", template["parameters"])
	}

	return params, nil
}

// DefaultValue returns the default value of a parameter and a boolean indicating if it was found.
func DefaultValue(parameter any) (any, bool) {
	if parameter == nil {
		return nil, false
	}

	param, ok := parameter.(map[string]any)
	if !ok {
		return nil, false
	}

	defaultValue, ok := param["defaultValue"]
	return defaultValue, ok
}
