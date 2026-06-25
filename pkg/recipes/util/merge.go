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

// ShallowMergeParameters merges two parameter maps with top-level key precedence from override.
// Nested objects are replaced entirely, not deep-merged (per the direct module support design).
func ShallowMergeParameters(base map[string]any, override map[string]any) map[string]any {
	if base == nil && override == nil {
		return nil
	}

	result := make(map[string]any)

	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}

	return result
}
